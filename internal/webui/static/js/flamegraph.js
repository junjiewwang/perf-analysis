/**
 * Flame Graph Module
 * Handles flame graph rendering, search, and filtering
 */

const FlameGraph = (function() {
    // Private state
    let flameChart = null;
    let flameGraphData = null;
    let originalFlameGraphData = null;
    let currentSearchTerm = '';
    let flameFilters = new Set();
    let isInverted = true;

    // System function patterns for filtering
    const SYSTEM_PATTERNS = {
        jvm: [
            /^java\./, /^javax\./, /^jdk\./, /^sun\./, /^com\.sun\./,
            /^org\.openjdk\./, /^java\.lang\./, /^java\.util\./,
            /^java\.io\./, /^java\.nio\./, /^java\.net\./,
            /^java\.security\./, /^java\.concurrent\./, /Unsafe\./,
            /^jdk\.internal\./
        ],
        gc: [
            /GC/i, /Garbage/i, /^G1/, /^ZGC/, /^Shenandoah/,
            /ParallelGC/, /ConcurrentMark/, /SafePoint/i, /safepoint/i,
            /VMThread/, /Reference.*Handler/
        ],
        native: [
            /^\[native\]/, /^libc\./, /^libpthread/, /^ld-linux/,
            /^__/, /^_Z/, /^std::/, /::operator/, /^pthread_/,
            /^malloc/, /^free$/, /^mmap/, /^munmap/, /^brk$/,
            /^clone$/, /^futex/, /^epoll_/, /^syscall/
        ],
        kernel: [
            /^\[kernel\]/, /^vmlinux/, /^do_syscall/, /^sys_/,
            /^__x64_sys/, /^entry_SYSCALL/, /^page_fault/,
            /^handle_mm_fault/, /^__schedule/, /^schedule$/,
            /^ret_from_fork/, /^irq_/, /^softirq/, /^ksoftirqd/,
            /^kworker/, /^rcu_/
        ],
        reflect: [
            /reflect/i, /Reflect/, /\$Proxy/, /CGLIB/, /ByteBuddy/,
            /javassist/, /MethodHandle/, /LambdaForm/,
            /GeneratedMethodAccessor/, /DelegatingMethodAccessor/,
            /NativeMethodAccessor/, /invoke0/, /invokespecial/,
            /invokevirtual/
        ]
    };

    // Transform flame graph data to d3-flamegraph format
    function transformFlameData(data) {
        if (!data) return null;
        const node = data.root || data;
        const result = {
            name: node.func || node.name || 'root',
            value: node.value || 0,
            children: []
        };
        if (node.children && Array.isArray(node.children)) {
            result.children = node.children.map(child => transformFlameData(child)).filter(c => c !== null);
        }
        return result;
    }

    // Deep clone flame graph data
    function deepCloneFlameData(node) {
        if (!node) return null;
        return {
            name: node.name,
            value: node.value,
            children: node.children ? node.children.map(c => deepCloneFlameData(c)) : []
        };
    }

    // Calculate flame graph statistics
    function calculateFlameStats(node, depth = 0, funcSet = new Set()) {
        if (!node) return { totalSamples: 0, uniqueFuncs: 0, maxDepth: 0 };
        funcSet.add(node.name);
        let maxDepth = depth;
        if (node.children && node.children.length > 0) {
            for (const child of node.children) {
                const childStats = calculateFlameStats(child, depth + 1, funcSet);
                maxDepth = Math.max(maxDepth, childStats.maxDepth);
            }
        }
        return {
            totalSamples: node.value || 0,
            uniqueFuncs: funcSet.size,
            maxDepth: maxDepth
        };
    }

    // Check if a function name matches system patterns
    function isSystemFunction(name, filterTypes) {
        if (!name || filterTypes.size === 0) return false;
        for (const filterType of filterTypes) {
            const patterns = SYSTEM_PATTERNS[filterType];
            if (patterns) {
                for (const pattern of patterns) {
                    if (pattern.test(name)) return true;
                }
            }
        }
        return false;
    }

    // Collapse filtered nodes
    function collapseFilteredNodes(node, filterTypes) {
        if (!node) return null;
        const cloned = deepCloneFlameData(node);
        const result = {
            name: cloned.name,
            value: cloned.value,
            children: []
        };
        if (cloned.children && cloned.children.length > 0) {
            result.children = processChildrenForCollapse(cloned.children, filterTypes);
        }
        recalculateValues(result);
        return result;
    }

    function processChildrenForCollapse(children, filterTypes) {
        const resultChildren = [];
        for (const child of children) {
            const isFiltered = isSystemFunction(child.name, filterTypes);
            if (isFiltered) {
                if (child.children && child.children.length > 0) {
                    const promotedChildren = processChildrenForCollapse(child.children, filterTypes);
                    resultChildren.push(...promotedChildren);
                }
            } else {
                const processedChild = {
                    name: child.name,
                    value: child.value || 0,
                    children: []
                };
                if (child.children && child.children.length > 0) {
                    processedChild.children = processChildrenForCollapse(child.children, filterTypes);
                }
                resultChildren.push(processedChild);
            }
        }
        return mergeFlameChildren(resultChildren);
    }

    function mergeFlameChildren(children) {
        if (!children || children.length === 0) return [];
        const nameMap = new Map();
        for (const child of children) {
            if (nameMap.has(child.name)) {
                const existing = nameMap.get(child.name);
                existing.value = (existing.value || 0) + (child.value || 0);
                if (child.children && child.children.length > 0) {
                    existing.children = existing.children || [];
                    for (const grandchild of child.children) {
                        existing.children.push(deepCloneFlameData(grandchild));
                    }
                }
            } else {
                nameMap.set(child.name, {
                    name: child.name,
                    value: child.value || 0,
                    children: child.children ? child.children.map(c => deepCloneFlameData(c)) : []
                });
            }
        }
        const result = [];
        for (const [name, node] of nameMap) {
            if (node.children.length > 0) {
                node.children = mergeFlameChildren(node.children);
            }
            result.push(node);
        }
        return result;
    }

    function recalculateValues(node) {
        if (!node.children || node.children.length === 0) {
            return node.value || 0;
        }
        let childSum = 0;
        for (const child of node.children) {
            childSum += recalculateValues(child);
        }
        node.value = Math.max(node.value || 0, childSum);
        return node.value;
    }

    function validateFlameData(node) {
        if (!node) return 0;
        if (typeof node.value !== 'number' || isNaN(node.value)) {
            node.value = 0;
        }
        if (!Array.isArray(node.children)) {
            node.children = [];
        }
        let childSum = 0;
        for (const child of node.children) {
            childSum += validateFlameData(child);
        }
        if (node.children.length > 0) {
            node.value = Math.max(node.value, childSum);
        }
        return node.value;
    }

    // Apply search highlight
    function applySearchHighlight(term) {
        if (!term) return 0;
        const termLower = term.toLowerCase();
        let matchCount = 0;
        d3.select('#flamegraph').selectAll('.d3-flame-graph g').each(function() {
            const g = d3.select(this);
            const titleEl = g.select('title');
            const name = titleEl.empty() ? '' : titleEl.text();
            if (name && name.toLowerCase().includes(termLower)) {
                g.classed('search-match', true);
                matchCount++;
            } else {
                g.classed('search-match', false);
            }
        });
        return matchCount;
    }

    // Public API
    return {
        init: function() {
            // Initialize event listeners
            const searchInput = document.getElementById('searchInput');
            if (searchInput) {
                searchInput.addEventListener('keyup', (e) => {
                    if (e.key === 'Enter') this.search();
                });
            }
        },

        async load(taskId) {
            const container = document.getElementById('flamegraph');
            container.innerHTML = '<div class="loading">Loading flame graph</div>';

            try {
                const data = await API.getFlameGraph(taskId);
                flameGraphData = transformFlameData(data);
                originalFlameGraphData = deepCloneFlameData(flameGraphData);

                if (!flameGraphData || flameGraphData.value === 0) {
                    container.innerHTML = '<div class="loading">No flame graph data available</div>';
                    return;
                }

                this.render();
            } catch (err) {
                console.error('Failed to load flame graph:', err);
                container.innerHTML = '<div class="loading">Failed to load flame graph: ' + err.message + '</div>';
            }
        },

        render() {
            const container = document.getElementById('flamegraph');
            const panel = document.getElementById('flamegraph-panel');

            container.innerHTML = '';

            if (!panel.classList.contains('active')) return;
            if (!flameGraphData) return;

            let width = container.clientWidth;
            if (width <= 0) {
                width = document.querySelector('.container').clientWidth - 40 || 1200;
            }

            // Calculate and display stats
            const stats = calculateFlameStats(flameGraphData);
            document.getElementById('flame-total-samples').textContent = stats.totalSamples.toLocaleString();
            document.getElementById('flame-unique-funcs').textContent = stats.uniqueFuncs.toLocaleString();
            document.getElementById('flame-max-depth').textContent = stats.maxDepth;

            // Create flame graph
            flameChart = flamegraph()
                .width(width)
                .cellHeight(22)
                .transitionDuration(400)
                .minFrameSize(1)
                .transitionEase(d3.easeCubicOut)
                .sort(true)
                .title('')
                .inverted(isInverted)
                .selfValue(false)
                .onClick(this.handleClick.bind(this));

            // Color scheme
            flameChart.setColorMapper(function(d, originalColor) {
                const name = d.data.name || '';
                let hash = 0;
                for (let i = 0; i < name.length; i++) {
                    hash = ((hash << 5) - hash) + name.charCodeAt(i);
                    hash |= 0;
                }
                const hue = 30 + (Math.abs(hash) % 30);
                const saturation = 70 + (Math.abs(hash >> 8) % 30);
                const lightness = 50 + (Math.abs(hash >> 16) % 20);
                return `hsl(${hue}, ${saturation}%, ${lightness}%)`;
            });

            try {
                d3.select('#flamegraph').datum(flameGraphData).call(flameChart);
            } catch (err) {
                console.error('Error rendering flame graph:', err);
                container.innerHTML = '<div class="loading" style="color: #e74c3c;">⚠️ Error rendering flame graph: ' + err.message + '</div>';
                return;
            }

            // Re-apply search after zoom
            d3.select('#flamegraph').on('click', () => {
                container.classList.add('zooming');
                setTimeout(() => {
                    container.classList.remove('zooming');
                    this.reapplySearch();
                }, 420);
            });

            // Handle resize
            let resizeTimeout;
            window.addEventListener('resize', () => {
                clearTimeout(resizeTimeout);
                resizeTimeout = setTimeout(() => {
                    if (flameChart && container.clientWidth > 0) {
                        flameChart.width(container.clientWidth);
                        d3.select('#flamegraph').datum(flameGraphData).call(flameChart);
                    }
                }, 200);
            });
        },

        handleClick(d) {
            const container = document.getElementById('flamegraph');
            container.classList.add('zooming');
        },

        search() {
            const term = document.getElementById('searchInput').value.trim();
            const badge = document.getElementById('searchResultBadge');

            if (!flameChart || !term) {
                badge.classList.add('hidden');
                this.clearSearch();
                return;
            }

            currentSearchTerm = term;
            const container = document.getElementById('flamegraph');
            container.classList.add('searching');

            flameChart.search(term);
            const matchCount = applySearchHighlight(term);

            badge.classList.remove('hidden', 'no-match');
            if (matchCount > 0) {
                badge.innerHTML = `✓ Found ${matchCount} matching frame${matchCount > 1 ? 's' : ''}`;
                badge.style.background = '#27ae60';
            } else {
                badge.classList.add('no-match');
                badge.innerHTML = `✗ No matches for "${Utils.escapeHtml(term)}"`;
                badge.style.background = '#e74c3c';
            }
        },

        reapplySearch() {
            if (currentSearchTerm) {
                const container = document.getElementById('flamegraph');
                container.classList.add('searching');
                requestAnimationFrame(() => {
                    const matchCount = applySearchHighlight(currentSearchTerm);
                    const badge = document.getElementById('searchResultBadge');
                    badge.classList.remove('hidden', 'no-match');
                    if (matchCount > 0) {
                        badge.innerHTML = `✓ Found ${matchCount} matching frame${matchCount > 1 ? 's' : ''}`;
                        badge.style.background = '#27ae60';
                    } else {
                        badge.classList.add('no-match');
                        badge.innerHTML = `✗ No matches in current view`;
                        badge.style.background = '#e74c3c';
                    }
                });
            }
        },

        clearSearch() {
            document.getElementById('searchInput').value = '';
            document.getElementById('searchResultBadge').classList.add('hidden');
            currentSearchTerm = '';
            const container = document.getElementById('flamegraph');
            container.classList.remove('searching');
            d3.select('#flamegraph').selectAll('.d3-flame-graph g.search-match')
                .classed('search-match', false);
            if (flameChart) flameChart.clear();
        },

        reset() {
            if (flameChart) {
                const container = document.getElementById('flamegraph');
                container.classList.add('zooming');
                flameChart.resetZoom();
                setTimeout(() => {
                    container.classList.remove('zooming');
                    this.reapplySearch();
                }, 420);
            }
        },

        toggleFilter(filterType) {
            const chip = document.querySelector(`#flameFilterSection .filter-chip[data-filter="${filterType}"]`);
            if (flameFilters.has(filterType)) {
                flameFilters.delete(filterType);
                chip.classList.remove('active');
            } else {
                flameFilters.add(filterType);
                chip.classList.add('active');
            }
            this.applyFilters();
        },

        applyFilters() {
            if (!originalFlameGraphData) return;
            const container = document.getElementById('flamegraph');

            if (flameFilters.size === 0) {
                container.classList.remove('filter-system');
                flameGraphData = deepCloneFlameData(originalFlameGraphData);
            } else {
                container.classList.add('filter-system');
                flameGraphData = collapseFilteredNodes(originalFlameGraphData, flameFilters);
            }

            if (flameGraphData) {
                container.innerHTML = '';
                flameChart = null;
                validateFlameData(flameGraphData);

                const hasChildren = flameGraphData.children && flameGraphData.children.length > 0;
                const hasValue = flameGraphData.value > 0;

                if (!hasValue && !hasChildren) {
                    container.innerHTML = '<div class="loading" style="color: #e74c3c;">⚠️ All functions were filtered out. Try removing some filters.</div>';
                    return;
                }

                this.render();
                if (currentSearchTerm) {
                    setTimeout(() => this.search(), 100);
                }
            }
        },

        searchFor(funcName) {
            const searchTerm = Utils.extractSearchTerm(funcName);
            document.getElementById('searchInput').value = searchTerm;
            setTimeout(() => this.search(), 100);
        },

        getData() {
            return flameGraphData;
        }
    };
})();

// Export for global access
window.FlameGraph = FlameGraph;
