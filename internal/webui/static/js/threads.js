/**
 * Thread Analysis Module
 * Provides thread-level profiling visualization with table layout,
 * search functionality, and integration with flame graph.
 * Supports CPU, allocation, and lock profiling data.
 * 
 * Data source: /api/flamegraph (unified flame graph API)
 */

const ThreadsPanel = (function() {
    'use strict';

    // Private state
    let state = {
        // Raw data from API
        flameGraphData: null,
        threadAnalysis: null,
        
        // Processed data
        threads: [],           // All threads (original)
        filteredThreads: [],   // After filtering
        threadGroups: [],
        topFunctions: [],
        totalSamples: 0,
        
        // UI state
        currentPage: 1,
        pageSize: 20,
        sortBy: 'samples',
        sortOrder: 'desc',
        searchTerm: '',
        isLoading: false
    };

    // DOM Elements cache
    let elements = {};

    // Debounce utility
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // Initialize the module
    function init() {
        cacheElements();
        bindEvents();
    }

    // Cache DOM elements
    function cacheElements() {
        elements = {
            tbody: document.getElementById('cpu-threads-tbody'),
            searchInput: document.getElementById('cpu-thread-search'),
            sortSelect: document.getElementById('cpu-thread-sort'),
            summaryText: document.getElementById('cpu-threads-summary'),
            pageInfo: document.getElementById('cpu-threads-page-info'),
            pageButtons: document.getElementById('cpu-threads-page-buttons')
        };
    }

    // Bind event listeners
    function bindEvents() {
        // Search input with debounce
        if (elements.searchInput) {
            elements.searchInput.addEventListener('input', debounce(handleSearch, 300));
            elements.searchInput.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') {
                    elements.searchInput.value = '';
                    handleSearch();
                } else if (e.key === 'Enter') {
                    handleSearch();
                }
            });
        }

        // Sort select
        if (elements.sortSelect) {
            elements.sortSelect.addEventListener('change', handleSortChange);
        }
    }

    // Load thread data from unified flame graph API
    async function load(taskId) {
        if (state.isLoading) return;

        state.isLoading = true;
        showLoading(true);

        try {
            // Determine the correct API type based on analysis type
            const analysisType = typeof App !== 'undefined' ? App.getAnalysisType() : 'cpu';
            const apiType = analysisType === 'alloc' ? 'memory' : 'cpu';
            const response = await fetch(`/api/flamegraph?type=${apiType}&task=${taskId || App.getCurrentTask()}`);
            if (!response.ok) throw new Error('Failed to fetch flame graph data');
            
            const data = await response.json();
            state.flameGraphData = data;
            state.threadAnalysis = data.thread_analysis;
            state.totalSamples = data.total_samples || 0;

            // Process thread data
            processThreadData();

            // Update summary
            updateSummary();

            // Render table
            render();

        } catch (error) {
            console.error('Failed to load CPU thread data:', error);
            showError('Failed to load CPU analysis data: ' + error.message);
        } finally {
            state.isLoading = false;
            showLoading(false);
        }
    }

    // Process thread data from flame graph
    function processThreadData() {
        if (!state.threadAnalysis) {
            state.threads = [];
            state.threadGroups = [];
            state.topFunctions = [];
            return;
        }

        const ta = state.threadAnalysis;

        // Process threads - convert to UI format
        state.threads = (ta.threads || []).map(t => ({
            tid: t.tid,
            thread_name: t.name,
            thread_group: t.group || extractThreadGroup(t.name),
            samples: t.samples,
            percentage: t.percentage,
            is_swapper: t.is_swapper || false,
            top_func: t.top_functions && t.top_functions.length > 0 ? t.top_functions[0].name : '',
            top_funcs: t.top_functions || [],
            call_stacks: t.top_call_stacks || []
        }));

        // Process thread groups
        state.threadGroups = (ta.thread_groups || []).map(g => ({
            group_name: g.name,
            thread_count: g.thread_count,
            total_samples: g.total_samples,
            percentage: g.percentage,
            top_thread: g.top_thread
        }));

        // Process top functions
        state.topFunctions = ta.top_functions || [];

        // Apply current filters and sorting
        applyFiltersAndSort();
    }

    // Extract thread group from thread name
    function extractThreadGroup(name) {
        if (!name) return 'unknown';
        
        const patterns = [
            /^(.*?)-\d+$/,
            /^(.*?)_\d+$/,
            /^(.*?)\[\d+\]$/,
            /^(.*?)#\d+$/,
            /^pool-\d+-(.*?)$/,
        ];

        for (const pattern of patterns) {
            const match = name.match(pattern);
            if (match) {
                return match[1];
            }
        }

        return name;
    }

    // Apply filters and sorting to threads
    function applyFiltersAndSort() {
        let filtered = [...state.threads];

        // Filter by search term
        if (state.searchTerm) {
            const term = state.searchTerm.toLowerCase();
            filtered = filtered.filter(t => 
                t.thread_name.toLowerCase().includes(term) ||
                (t.top_func && t.top_func.toLowerCase().includes(term)) ||
                (t.thread_group && t.thread_group.toLowerCase().includes(term))
            );
        }

        // Sort
        filtered.sort((a, b) => {
            let cmp = 0;
            switch (state.sortBy) {
                case 'samples':
                    cmp = a.samples - b.samples;
                    break;
                case 'name':
                    cmp = a.thread_name.localeCompare(b.thread_name);
                    break;
                case 'tid':
                    cmp = a.tid - b.tid;
                    break;
                case 'percentage':
                    cmp = a.percentage - b.percentage;
                    break;
                default:
                    cmp = a.samples - b.samples;
            }
            return state.sortOrder === 'desc' ? -cmp : cmp;
        });

        state.filteredThreads = filtered;
    }

    // Update summary text
    function updateSummary() {
        if (elements.summaryText) {
            const ta = state.threadAnalysis;
            if (ta) {
                elements.summaryText.textContent = `${Utils.formatNumber(state.totalSamples)} samples, ${ta.active_threads || state.threads.length} threads, ${ta.unique_functions || 0} functions`;
            }
        }
    }

    // Render the table
    function render() {
        if (!elements.tbody) {
            cacheElements();
        }
        if (!elements.tbody) return;

        const startIdx = (state.currentPage - 1) * state.pageSize;
        const endIdx = Math.min(startIdx + state.pageSize, state.filteredThreads.length);
        const pageThreads = state.filteredThreads.slice(startIdx, endIdx);

        if (pageThreads.length === 0) {
            elements.tbody.innerHTML = `
                <tr><td colspan="7" class="px-6 py-8 text-center text-theme-muted">
                    ${state.searchTerm ? 'No threads match your search criteria' : 'No thread data available'}
                </td></tr>
            `;
        } else {
            elements.tbody.innerHTML = pageThreads.map((thread, idx) => 
                renderThreadRow(thread, startIdx + idx)
            ).join('');
        }

        renderPagination();
    }

    // Render a single thread row
    function renderThreadRow(thread, index) {
        const percentage = thread.percentage.toFixed(2);
        const barWidth = Math.min(100, thread.percentage);
        const topFunc = thread.top_func ? Utils.stripAllocationMarker(thread.top_func) : '-';
        const truncatedFunc = topFunc.length > 50 ? topFunc.substring(0, 47) + '...' : topFunc;
        
        // Escape thread name for use in onclick handlers
        const escapedThreadName = Utils.escapeHtml(thread.thread_name).replace(/'/g, "\\'");
        const escapedTopFunc = Utils.escapeHtml(thread.top_func || '').replace(/'/g, "\\'");

        return `
            <tr class="table-row-hover transition-colors">
                <td class="px-6 py-4 text-sm text-theme-muted font-medium">${index + 1}</td>
                <td class="px-6 py-4">
                    <div class="flex flex-col">
                        <span class="font-mono text-sm text-theme-base" title="${Utils.escapeHtml(thread.thread_name)}">${Utils.escapeHtml(thread.thread_name)}</span>
                        ${thread.thread_group && thread.thread_group !== thread.thread_name ? 
                            `<span class="text-xs text-theme-muted mt-0.5">${Utils.escapeHtml(thread.thread_group)}</span>` : ''}
                    </div>
                </td>
                <td class="px-6 py-4 text-sm text-theme-secondary font-mono text-right">${thread.tid}</td>
                <td class="px-6 py-4 text-sm font-semibold text-theme-base text-right">${Utils.formatNumber(thread.samples)}</td>
                <td class="px-6 py-4">
                    <div class="flex items-center gap-3">
                        <div class="flex-1 h-2 bg-theme-muted rounded-full overflow-hidden max-w-[120px]">
                            <div class="h-full thread-progress-bar rounded-full transition-all duration-300" style="width: ${barWidth}%"></div>
                        </div>
                        <span class="text-sm font-semibold text-theme-base w-16 text-right">${percentage}%</span>
                    </div>
                </td>
                <td class="px-6 py-4">
                    <span class="font-mono text-sm text-theme-secondary cursor-pointer hover:text-primary hover:underline" 
                          title="${Utils.escapeHtml(thread.top_func || '')}"
                          onclick="ThreadsPanel.searchTopFuncInFlameGraph('${escapedTopFunc}')">${Utils.escapeHtml(truncatedFunc)}</span>
                </td>
                <td class="px-6 py-4 text-center">
                    <div class="flex items-center justify-center gap-2">
                        <button class="w-8 h-8 rounded-lg bg-gradient-to-r from-orange-500 to-red-500 text-white flex items-center justify-center hover:scale-110 transition-transform" 
                                title="Search thread in Flame Graph" 
                                onclick="ThreadsPanel.searchThreadInFlameGraph('${escapedThreadName}')">🔥</button>
                        <button class="w-8 h-8 rounded-lg bg-gradient-to-r from-primary to-secondary text-white flex items-center justify-center hover:scale-110 transition-transform" 
                                title="View thread details" 
                                onclick="ThreadsPanel.showThreadDetail(${thread.tid})">📋</button>
                    </div>
                </td>
            </tr>
        `;
    }

    // Render pagination
    function renderPagination() {
        const totalPages = Math.ceil(state.filteredThreads.length / state.pageSize);
        const startIdx = (state.currentPage - 1) * state.pageSize + 1;
        const endIdx = Math.min(state.currentPage * state.pageSize, state.filteredThreads.length);

        // Update page info
        if (elements.pageInfo) {
            if (state.filteredThreads.length === 0) {
                elements.pageInfo.textContent = 'No threads to display';
            } else {
                elements.pageInfo.textContent = `Showing ${startIdx}-${endIdx} of ${state.filteredThreads.length} threads`;
            }
        }

        // Update page buttons
        if (elements.pageButtons) {
            if (totalPages <= 1) {
                elements.pageButtons.innerHTML = '';
                return;
            }

            let pages = [];
            const maxVisiblePages = 7;
            
            if (totalPages <= maxVisiblePages) {
                pages = Array.from({ length: totalPages }, (_, i) => i + 1);
            } else {
                pages.push(1);
                if (state.currentPage > 3) pages.push('...');
                
                const start = Math.max(2, state.currentPage - 1);
                const end = Math.min(totalPages - 1, state.currentPage + 1);
                
                for (let i = start; i <= end; i++) {
                    if (!pages.includes(i)) pages.push(i);
                }
                
                if (state.currentPage < totalPages - 2) pages.push('...');
                if (!pages.includes(totalPages)) pages.push(totalPages);
            }

            elements.pageButtons.innerHTML = `
                <button class="px-3 py-1.5 text-sm rounded ${state.currentPage === 1 ? 'text-theme-muted cursor-not-allowed' : 'text-theme-base hover:bg-theme-hover'}" 
                        ${state.currentPage === 1 ? 'disabled' : ''} 
                        onclick="ThreadsPanel.goToPage(${state.currentPage - 1})">← Prev</button>
                ${pages.map(p => p === '...' 
                    ? '<span class="px-2 text-theme-muted">...</span>'
                    : `<button class="w-8 h-8 text-sm rounded ${p === state.currentPage ? 'bg-primary text-white' : 'text-theme-base hover:bg-theme-hover'}" 
                              onclick="ThreadsPanel.goToPage(${p})">${p}</button>`
                ).join('')}
                <button class="px-3 py-1.5 text-sm rounded ${state.currentPage === totalPages ? 'text-theme-muted cursor-not-allowed' : 'text-theme-base hover:bg-theme-hover'}" 
                        ${state.currentPage === totalPages ? 'disabled' : ''} 
                        onclick="ThreadsPanel.goToPage(${state.currentPage + 1})">Next →</button>
            `;
        }
    }

    // Event handlers
    function handleSearch() {
        state.searchTerm = elements.searchInput?.value || '';
        state.currentPage = 1;
        applyFiltersAndSort();
        render();
    }

    function handleSortChange() {
        const value = elements.sortSelect?.value || 'samples-desc';
        const [sortBy, sortOrder] = value.split('-');
        state.sortBy = sortBy;
        state.sortOrder = sortOrder;
        state.currentPage = 1;
        applyFiltersAndSort();
        render();
    }

    // Show loading state
    function showLoading(show) {
        if (elements.tbody && show) {
            elements.tbody.innerHTML = `
                <tr><td colspan="7" class="px-6 py-8 text-center text-theme-muted">
                    <div class="animate-spin text-2xl mb-2">⏳</div>
                    <p>Loading thread analysis data...</p>
                </td></tr>
            `;
        }
    }

    // Show error state
    function showError(message) {
        if (elements.tbody) {
            elements.tbody.innerHTML = `
                <tr><td colspan="7" class="px-6 py-8 text-center" style="color: rgb(var(--color-danger));">
                    <p>⚠️ ${Utils.escapeHtml(message)}</p>
                </td></tr>
            `;
        }
    }

    // Get thread by TID
    function getThreadByTid(tid) {
        return state.threads.find(t => t.tid === tid);
    }

    // Show thread detail modal
    function showThreadDetailModal(thread) {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        const bgCard = isDark ? '#1f2937' : 'white';
        const bgMuted = isDark ? '#374151' : '#f9fafb';
        const textBase = isDark ? '#f3f4f6' : '#1f2937';
        const textSecondary = isDark ? '#9ca3af' : '#6b7280';
        const textMuted = isDark ? '#6b7280' : '#9ca3af';
        const borderColor = isDark ? '#4b5563' : '#e5e7eb';
        const accentColor = isDark ? '#2dd4bf' : '#0d9488';
        const btnSecondaryBg = isDark ? '#374151' : '#f3f4f6';
        const btnSecondaryText = isDark ? '#f3f4f6' : '#374151';
        
        const modal = document.createElement('div');
        modal.className = 'modal-overlay';
        modal.innerHTML = `
            <div class="modal-content" style="background: ${bgCard}; max-width: 700px; border-radius: 12px; color: ${textBase};">
                <div class="modal-header" style="padding: 16px 20px; border-bottom: 1px solid ${borderColor}; display: flex; justify-content: space-between; align-items: center;">
                    <h3 style="margin: 0; font-size: 16px; color: ${textBase};">🧵 ${Utils.escapeHtml(thread.thread_name)}</h3>
                    <button class="modal-close" onclick="this.closest('.modal-overlay').remove()" style="background: none; border: none; font-size: 24px; cursor: pointer; color: ${textSecondary};">×</button>
                </div>
                <div class="modal-body" style="padding: 20px;">
                    <div style="display: flex; gap: 24px; margin-bottom: 20px; padding-bottom: 16px; border-bottom: 1px solid ${borderColor};">
                        <div>
                            <span style="color: ${textSecondary}; font-size: 12px;">TID</span>
                            <div style="font-weight: 600; font-family: monospace; color: ${textBase};">${thread.tid}</div>
                        </div>
                        <div>
                            <span style="color: ${textSecondary}; font-size: 12px;">Samples</span>
                            <div style="font-weight: 600; color: ${textBase};">${Utils.formatNumber(thread.samples)}</div>
                        </div>
                        <div>
                            <span style="color: ${textSecondary}; font-size: 12px;">Percentage</span>
                            <div style="font-weight: 600; color: ${accentColor};">${thread.percentage.toFixed(2)}%</div>
                        </div>
                        <div>
                            <span style="color: ${textSecondary}; font-size: 12px;">Thread Group</span>
                            <div style="font-weight: 500; color: ${textBase};">${Utils.escapeHtml(thread.thread_group || '-')}</div>
                        </div>
                    </div>
                    <div style="margin-bottom: 16px;">
                        <h4 style="font-size: 14px; font-weight: 600; color: ${textBase}; margin-bottom: 12px;">Top Functions</h4>
                        <table style="width: 100%; border-collapse: collapse; font-size: 13px;">
                            <thead>
                                <tr style="background: ${bgMuted};">
                                    <th style="padding: 8px 12px; text-align: left; font-weight: 600; color: ${textSecondary};">Function</th>
                                    <th style="padding: 8px 12px; text-align: right; font-weight: 600; color: ${textSecondary}; width: 80px;">Samples</th>
                                    <th style="padding: 8px 12px; text-align: right; font-weight: 600; color: ${textSecondary}; width: 80px;">%</th>
                                </tr>
                            </thead>
                            <tbody>
                                ${thread.top_funcs?.slice(0, 10).map(f => `
                                    <tr style="border-bottom: 1px solid ${borderColor};">
                                        <td style="padding: 8px 12px; font-family: monospace; max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: ${textBase};" title="${Utils.escapeHtml(f.name)}">${Utils.escapeHtml(Utils.stripAllocationMarker(f.name))}</td>
                                        <td style="padding: 8px 12px; text-align: right; font-family: monospace; color: ${textBase};">${Utils.formatNumber(f.samples)}</td>
                                        <td style="padding: 8px 12px; text-align: right; color: ${accentColor}; font-weight: 500;">${f.percentage.toFixed(2)}%</td>
                                    </tr>
                                `).join('') || `<tr><td colspan="3" style="padding: 16px; text-align: center; color: ${textMuted};">No function data</td></tr>`}
                            </tbody>
                        </table>
                    </div>
                    <div style="display: flex; gap: 8px; justify-content: flex-end; padding-top: 16px; border-top: 1px solid ${borderColor};">
                        <button onclick="ThreadsPanel.searchThreadInFlameGraph('${Utils.escapeHtml(thread.thread_name).replace(/'/g, "\\'")}'); this.closest('.modal-overlay').remove();" 
                                style="padding: 8px 16px; background: linear-gradient(135deg, #f97316, #ef4444); color: white; border: none; border-radius: 6px; cursor: pointer; font-size: 13px;">
                            🔥 Search in Flame Graph
                        </button>
                        <button onclick="this.closest('.modal-overlay').remove();" 
                                style="padding: 8px 16px; background: ${btnSecondaryBg}; color: ${btnSecondaryText}; border: none; border-radius: 6px; cursor: pointer; font-size: 13px;">
                            Close
                        </button>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(modal);
        modal.addEventListener('click', (e) => {
            if (e.target === modal) modal.remove();
        });
    }

    // Public API
    return {
        init,
        load,

        // Refresh data
        refresh(taskId) {
            state.threads = [];
            state.filteredThreads = [];
            state.threadGroups = [];
            state.currentPage = 1;
            state.searchTerm = '';
            if (elements.searchInput) elements.searchInput.value = '';
            load(taskId || (typeof App !== 'undefined' ? App.getCurrentTask() : ''));
        },

        // Filter by thread group name
        filterByGroup(groupName) {
            if (!groupName) return;
            
            // Set search term to the group name
            state.searchTerm = groupName;
            state.currentPage = 1;
            
            // Update search input if it exists
            if (elements.searchInput) {
                elements.searchInput.value = groupName;
            }
            
            // Apply filters and re-render
            applyFiltersAndSort();
            render();
        },

        // Pagination
        goToPage(page) {
            const totalPages = Math.ceil(state.filteredThreads.length / state.pageSize);
            if (page < 1 || page > totalPages) return;
            state.currentPage = page;
            render();
        },

        // Thread actions
        showThreadDetail(tid) {
            const thread = getThreadByTid(tid);
            if (thread) {
                showThreadDetailModal(thread);
            }
        },

        // Search thread name in flame graph
        // This searches for the thread name pattern in the flame graph
        searchThreadInFlameGraph(threadName) {
            if (!threadName) return;
            
            // Switch to flame graph panel and search
            if (typeof App !== 'undefined' && App.searchInFlameGraph) {
                App.searchInFlameGraph(threadName);
            }
        },

        // Search top function in flame graph
        searchTopFuncInFlameGraph(funcName) {
            if (!funcName) return;
            
            // Strip allocation marker and search
            const cleanName = Utils.stripAllocationMarker(funcName);
            if (typeof App !== 'undefined' && App.searchInFlameGraph) {
                App.searchInFlameGraph(cleanName);
            }
        },

        // State access
        getState() {
            return { ...state };
        },

        // Re-render the table (for theme changes)
        rerender() {
            render();
        }
    };
})();

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', () => {
    ThreadsPanel.init();
    
    // Listen for theme changes and re-render
    if (typeof ThemeManager !== 'undefined') {
        ThemeManager.onChange(() => {
            // Re-render table to update theme-aware colors
            ThreadsPanel.rerender();
        });
    }
});

// Export for global access (with backward compatibility alias)
window.ThreadsPanel = ThreadsPanel;
window.CPUThreads = ThreadsPanel; // Backward compatibility
