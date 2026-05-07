/**
 * Heap Dominator Tree Module
 * Dominator Tree 视图：类似 Eclipse MAT 的可展开支配树
 * 
 * 功能：
 * - 按 Retained Size 排序的支配树节点
 * - 懒加载子节点（点击展开时才请求）
 * - 支配链路径显示
 * - 搜索和排序功能
 */

const HeapDominatorTree = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================

    // Tree state: Map<nodeKey, { expanded, children, loaded, hasChildren }>
    let treeState = new Map();
    let rootNodes = [];
    let currentTaskId = '';
    let isLoading = false;
    let currentSort = 'retained';

    // ============================================
    // 工具函数
    // ============================================

    function formatBytes(bytes) {
        if (typeof Utils !== 'undefined' && Utils.formatBytes) {
            return Utils.formatBytes(bytes);
        }
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    function escapeHtml(str) {
        if (!str) return '';
        if (typeof Utils !== 'undefined' && Utils.escapeHtml) {
            return Utils.escapeHtml(str);
        }
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    function formatClassName(fullName) {
        if (!fullName) return '<span class="text-gray-400">&lt;unknown&gt;</span>';
        const lastDot = fullName.lastIndexOf('.');
        if (lastDot === -1) {
            return `<span class="font-semibold text-gray-800 dark:text-gray-200">${escapeHtml(fullName)}</span>`;
        }
        const packagePart = fullName.substring(0, lastDot + 1);
        const simpleName = fullName.substring(lastDot + 1);
        return `<span class="text-gray-400 dark:text-gray-500 text-xs">${escapeHtml(packagePart)}</span><span class="font-semibold text-gray-800 dark:text-gray-200">${escapeHtml(simpleName)}</span>`;
    }

    function formatObjectId(objId) {
        if (!objId) return '';
        return String(objId).replace('0x', '').substring(0, 8);
    }

    function renderExpandIcon(hasChildren, isExpanded) {
        if (!hasChildren) {
            return '<span class="w-4 h-4 inline-block"></span>';
        }
        const iconClass = isExpanded ? 'rotate-90' : '';
        return `<svg class="w-4 h-4 inline-block transition-transform duration-200 ${iconClass} text-gray-500 dark:text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
        </svg>`;
    }

    // ============================================
    // API 调用
    // ============================================

    async function fetchDominatorChildren(objectId, topN = 50) {
        try {
            let url = `/api/refgraph/dominator-tree?top=${topN}&sort=${currentSort}`;
            if (currentTaskId) {
                url += `&task=${encodeURIComponent(currentTaskId)}`;
            }
            if (objectId) {
                url += `&id=${encodeURIComponent(objectId)}`;
            }
            const response = await fetch(url);
            if (!response.ok) {
                console.warn(`[DominatorTree] Failed to fetch children: ${response.status}`);
                return [];
            }
            return await response.json();
        } catch (error) {
            console.error('[DominatorTree] Error fetching children:', error);
            return [];
        }
    }

    async function fetchDominatorPath(objectId) {
        try {
            let url = `/api/refgraph/dominator-path?id=${encodeURIComponent(objectId)}`;
            if (currentTaskId) {
                url += `&task=${encodeURIComponent(currentTaskId)}`;
            }
            const response = await fetch(url);
            if (!response.ok) return [];
            return await response.json();
        } catch (error) {
            console.error('[DominatorTree] Error fetching path:', error);
            return [];
        }
    }

    // ============================================
    // 渲染
    // ============================================

    function renderTreeNode(node, depth = 0, parentKey = '') {
        const nodeKey = parentKey ? `${parentKey}>${node.object_id}` : node.object_id;
        const state = treeState.get(nodeKey) || { expanded: false, children: [], loaded: false };
        const hasChildren = node.has_children;
        const isExpanded = state.expanded;

        const indent = depth * 20;
        const retainedSize = node.retained_size || 0;
        const shallowSize = node.shallow_size || 0;

        // Size bar - relative to parent's retained size
        const maxRetained = rootNodes.length > 0 ? rootNodes[0].retained_size : retainedSize;
        const retainedPercent = maxRetained > 0 ? (retainedSize / maxRetained * 100) : 0;

        let html = `
            <div class="dom-tree-row hover:bg-gray-100 dark:hover:bg-gray-700 border-b border-gray-100 dark:border-gray-700" data-node-key="${escapeHtml(nodeKey)}" data-depth="${depth}">
                <div class="flex items-center py-1.5 px-3 cursor-pointer gap-1" style="padding-left: ${indent + 12}px" onclick="HeapDominatorTree.toggleNode('${escapeHtml(nodeKey)}', '${escapeHtml(node.object_id)}')">
                    <span class="flex-shrink-0">${renderExpandIcon(hasChildren, isExpanded)}</span>
                    ${node.is_gc_root ? '<span class="flex-shrink-0 text-[9px] px-1 py-0.5 bg-green-100 dark:bg-green-900 text-green-700 dark:text-green-300 rounded font-medium">ROOT</span>' : ''}
                    <span class="flex-1 font-mono text-[11px] truncate" title="${escapeHtml(node.class_name || '')}">${formatClassName(node.class_name)}</span>
                    <span class="flex-shrink-0 text-[9px] text-gray-400 dark:text-gray-500 mr-2">@${formatObjectId(node.object_id)}</span>
                    <span class="flex-shrink-0 w-16 text-right text-[10px] text-gray-500 dark:text-gray-400">${formatBytes(shallowSize)}</span>
                    <div class="flex-shrink-0 w-32 flex items-center gap-1 ml-2">
                        <div class="flex-1 h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
                            <div class="h-full bg-gradient-to-r from-orange-500 to-red-500 rounded-full transition-all" style="width: ${Math.min(retainedPercent, 100)}%"></div>
                        </div>
                        <span class="text-[11px] font-semibold ${retainedSize > 1024 * 1024 ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-300'} w-16 text-right">${formatBytes(retainedSize)}</span>
                    </div>
                    <button class="flex-shrink-0 ml-1 px-1 py-0.5 text-[9px] bg-indigo-50 dark:bg-indigo-900 text-indigo-600 dark:text-indigo-300 rounded hover:bg-indigo-100 dark:hover:bg-indigo-800 transition-colors" onclick="event.stopPropagation(); HeapDominatorTree.showPath('${escapeHtml(node.object_id)}')" title="Show dominator path">
                        Path
                    </button>
                </div>
            </div>`;

        // Render children if expanded
        if (isExpanded && state.children && state.children.length > 0) {
            for (const child of state.children) {
                html += renderTreeNode(child, depth + 1, nodeKey);
            }
        } else if (isExpanded && !state.loaded) {
            html += `
                <div class="flex items-center py-2 px-3 text-gray-400 dark:text-gray-500 text-xs" style="padding-left: ${indent + 36}px">
                    <div class="animate-spin rounded-full h-3 w-3 border-2 border-gray-300 border-t-blue-500 mr-2"></div>
                    Loading dominated objects...
                </div>`;
        }

        return html;
    }

    function renderTree() {
        const container = document.getElementById('dominatorTreeList');
        if (!container) return;

        if (rootNodes.length === 0) {
            container.innerHTML = `
                <div class="text-center py-12 text-gray-500 dark:text-gray-400">
                    <svg class="w-12 h-12 mx-auto mb-3 text-gray-300 dark:text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h14a1 1 0 011 1v2a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM4 13a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H5a1 1 0 01-1-1v-6zM16 13a1 1 0 011-1h2a1 1 0 011 1v6a1 1 0 01-1 1h-2a1 1 0 01-1-1v-6z"></path>
                    </svg>
                    <p class="text-sm font-medium">No dominator tree data available</p>
                    <p class="text-xs mt-1">Please ensure the heap dump has been analyzed with dominator computation</p>
                </div>`;
            return;
        }

        // Table header
        let html = `
            <div class="sticky top-0 z-10 bg-gray-100 dark:bg-gray-800 border-b border-gray-300 dark:border-gray-600 flex items-center py-1.5 px-3 text-[10px] text-gray-600 dark:text-gray-400 font-semibold uppercase tracking-wide">
                <span class="flex-1 pl-6">Object (Dominator Tree)</span>
                <span class="w-16 text-right">Shallow</span>
                <span class="w-32 text-right pr-1 ml-2">Retained Size</span>
                <span class="w-8"></span>
            </div>
            <div class="divide-y divide-gray-50 dark:divide-gray-700">`;

        for (const node of rootNodes) {
            html += renderTreeNode(node, 0, '');
        }

        html += '</div>';
        container.innerHTML = html;
    }

    function renderSummary() {
        const container = document.getElementById('dominatorTreeSummary');
        if (!container) return;

        const totalRetained = rootNodes.reduce((sum, n) => sum + (n.retained_size || 0), 0);
        const gcRootCount = rootNodes.filter(n => n.is_gc_root).length;

        container.innerHTML = `
            <div class="flex flex-wrap items-center gap-3">
                <div class="flex items-center gap-2 px-3 py-1.5 bg-indigo-50 dark:bg-indigo-900/50 rounded-lg border border-indigo-200 dark:border-indigo-700">
                    <span class="text-lg font-bold text-indigo-600 dark:text-indigo-400">${rootNodes.length}</span>
                    <span class="text-xs text-indigo-500 dark:text-indigo-400">Top-Level Objects</span>
                </div>
                <div class="flex items-center gap-2 px-3 py-1.5 bg-red-50 dark:bg-red-900/50 rounded-lg border border-red-200 dark:border-red-700">
                    <span class="text-lg font-bold text-red-600 dark:text-red-400">${formatBytes(totalRetained)}</span>
                    <span class="text-xs text-red-500 dark:text-red-400">Total Retained</span>
                </div>
                <div class="flex items-center gap-2 px-3 py-1.5 bg-green-50 dark:bg-green-900/50 rounded-lg border border-green-200 dark:border-green-700">
                    <span class="text-lg font-bold text-green-600 dark:text-green-400">${gcRootCount}</span>
                    <span class="text-xs text-green-500 dark:text-green-400">GC Roots</span>
                </div>
            </div>`;
    }

    // ============================================
    // 公共方法
    // ============================================

    function init() {
        console.log('[HeapDominatorTree] Initializing...');
    }

    async function load(taskId) {
        currentTaskId = taskId || '';
        isLoading = true;

        const container = document.getElementById('dominatorTreeList');
        if (container) {
            container.innerHTML = `
                <div class="text-center py-12">
                    <div class="inline-block animate-spin rounded-full h-8 w-8 border-4 border-indigo-500 border-t-transparent"></div>
                    <p class="mt-4 text-gray-500 dark:text-gray-400">Loading dominator tree...</p>
                </div>`;
        }

        try {
            // Fetch root-level nodes (dominator == -1, i.e., dominated by virtual super root)
            const children = await fetchDominatorChildren('', 100);
            rootNodes = Array.isArray(children) ? children : [];

            // Initialize tree state
            treeState.clear();
            for (const node of rootNodes) {
                treeState.set(node.object_id, {
                    expanded: false,
                    children: [],
                    loaded: false,
                    hasChildren: node.has_children
                });
            }

            console.log('[HeapDominatorTree] Loaded', rootNodes.length, 'root nodes');
            renderSummary();
            renderTree();
        } catch (error) {
            console.error('[HeapDominatorTree] Failed to load:', error);
            if (container) {
                container.innerHTML = `
                    <div class="text-center py-12 text-red-500">
                        <p class="text-lg font-medium">Failed to load dominator tree</p>
                        <p class="text-sm mt-2">${escapeHtml(error.message)}</p>
                    </div>`;
            }
        } finally {
            isLoading = false;
        }
    }

    async function toggleNode(nodeKey, objectId) {
        let state = treeState.get(nodeKey);
        if (!state) {
            state = { expanded: false, children: [], loaded: false, hasChildren: true };
            treeState.set(nodeKey, state);
        }

        if (state.expanded) {
            state.expanded = false;
        } else {
            state.expanded = true;
            if (!state.loaded) {
                // Show loading state
                renderTree();
                // Fetch children
                const children = await fetchDominatorChildren(objectId, 50);
                state.children = Array.isArray(children) ? children : [];
                state.loaded = true;

                // Initialize child states
                for (const child of state.children) {
                    const childKey = `${nodeKey}>${child.object_id}`;
                    if (!treeState.has(childKey)) {
                        treeState.set(childKey, {
                            expanded: false,
                            children: [],
                            loaded: false,
                            hasChildren: child.has_children
                        });
                    }
                }
            }
        }

        treeState.set(nodeKey, state);
        renderTree();
    }

    async function showPath(objectId) {
        const path = await fetchDominatorPath(objectId);
        if (!path || path.length === 0) {
            alert('No dominator path found for this object.');
            return;
        }

        // Create modal
        const modal = document.createElement('div');
        modal.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50';
        modal.onclick = (e) => { if (e.target === modal) modal.remove(); };

        let pathHtml = '<div class="space-y-1 ml-2 border-l-2 border-indigo-200 dark:border-indigo-700 pl-4">';
        path.forEach((node, idx) => {
            const isLast = idx === path.length - 1;
            const isFirst = idx === 0;
            pathHtml += `
                <div class="flex items-center gap-2 py-1.5 ${isLast ? 'font-semibold' : ''}">
                    <span class="w-5 h-5 flex-shrink-0 flex items-center justify-center rounded-full text-[10px] font-bold ${isFirst ? 'bg-green-100 text-green-600' : isLast ? 'bg-red-100 text-red-600' : 'bg-gray-100 text-gray-600'}">${idx}</span>
                    <span class="font-mono text-sm ${isLast ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-300'}">${escapeHtml(node.class_name || '<unknown>')}</span>
                    <span class="text-[10px] text-gray-400">@${formatObjectId(node.object_id)}</span>
                    <span class="text-xs text-gray-400 ml-auto">${formatBytes(node.retained_size || 0)}</span>
                </div>`;
        });
        pathHtml += '</div>';

        modal.innerHTML = `
            <div class="bg-white dark:bg-gray-800 rounded-xl shadow-2xl max-w-2xl w-full mx-4 max-h-[80vh] flex flex-col" onclick="event.stopPropagation()">
                <div class="flex items-center justify-between px-6 py-4 border-b border-gray-200 dark:border-gray-700">
                    <h3 class="text-lg font-semibold text-gray-800 dark:text-gray-200">🌲 Dominator Path</h3>
                    <button class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" onclick="this.closest('.fixed').remove()">
                        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                        </svg>
                    </button>
                </div>
                <div class="flex-1 overflow-auto px-6 py-4">
                    <p class="text-sm text-gray-500 dark:text-gray-400 mb-4">
                        Path from virtual root to target object (${path.length} nodes):
                    </p>
                    ${pathHtml}
                </div>
            </div>`;

        document.body.appendChild(modal);
    }

    function changeSort(sortBy) {
        currentSort = sortBy;
        load(currentTaskId);
    }

    function expandAll() {
        // Only expand first level
        for (const node of rootNodes) {
            const state = treeState.get(node.object_id);
            if (state && !state.expanded) {
                toggleNode(node.object_id, node.object_id);
            }
        }
    }

    function collapseAll() {
        for (const [key, state] of treeState) {
            state.expanded = false;
        }
        renderTree();
    }

    function refresh() {
        treeState.clear();
        rootNodes = [];
        load(currentTaskId);
    }

    // ============================================
    // 模块注册
    // ============================================

    const module = {
        init,
        load,
        toggleNode,
        showPath,
        changeSort,
        expandAll,
        collapseAll,
        refresh
    };

    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('dominatorTree', module);
    }

    return module;
})();

window.HeapDominatorTree = HeapDominatorTree;
