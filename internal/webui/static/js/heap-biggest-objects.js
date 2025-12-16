/**
 * Heap Biggest Objects Module
 * 最大对象分析模块：展示堆中占用内存最大的对象及其详细信息
 * 
 * 功能：
 * - 按 Retained Size 排序的对象列表（过滤基础类型）
 * - 多层级树状展开（类似 IDEA）
 * - 对象字段详情展示
 * - GC Root 路径展示
 * - 搜索和过滤功能
 * - 排序功能
 */

const HeapBiggestObjects = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let biggestObjects = [];
    let filteredObjects = [];
    let currentSort = { field: 'retained', asc: false };
    // Tree state: Map<objectId, { expanded: bool, children: [], loaded: bool }>
    let treeState = new Map();
    let isLoading = false;

    // ============================================
    // 私有方法
    // ============================================

    /**
     * 格式化字节大小
     */
    function formatBytes(bytes) {
        if (typeof Utils !== 'undefined' && Utils.formatBytes) {
            return Utils.formatBytes(bytes);
        }
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    /**
     * 格式化数字
     */
    function formatNumber(num) {
        if (typeof Utils !== 'undefined' && Utils.formatNumber) {
            return Utils.formatNumber(num);
        }
        return num.toLocaleString();
    }

    /**
     * HTML 转义
     */
    function escapeHtml(str) {
        if (typeof Utils !== 'undefined' && Utils.escapeHtml) {
            return Utils.escapeHtml(str);
        }
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    /**
     * 格式化类名（IDEA 风格）
     */
    function formatClassName(fullName, highlightSimple = true) {
        if (!fullName) return '';
        const lastDot = fullName.lastIndexOf('.');
        if (lastDot === -1) {
            return highlightSimple 
                ? `<span class="font-semibold text-gray-800">${escapeHtml(fullName)}</span>`
                : escapeHtml(fullName);
        }
        const packagePart = fullName.substring(0, lastDot + 1);
        const simpleName = fullName.substring(lastDot + 1);
        if (highlightSimple) {
            return `<span class="text-gray-400 text-xs">${escapeHtml(packagePart)}</span><span class="font-semibold text-gray-800">${escapeHtml(simpleName)}</span>`;
        }
        return escapeHtml(fullName);
    }

    /**
     * 获取简短类名
     */
    function getSimpleClassName(fullName) {
        if (!fullName) return '';
        const lastDot = fullName.lastIndexOf('.');
        return lastDot === -1 ? fullName : fullName.substring(lastDot + 1);
    }

    /**
     * 格式化对象ID（移除0x前缀显示）
     */
    function formatObjectId(objId) {
        if (!objId) return '';
        return String(objId).replace('0x', '');
    }

    /**
     * 渲染树节点展开图标
     */
    function renderExpandIcon(hasChildren, isExpanded) {
        if (!hasChildren) {
            return '<span class="w-4 h-4 inline-block"></span>';
        }
        const iconClass = isExpanded ? 'rotate-90' : '';
        return `<svg class="w-4 h-4 inline-block transition-transform duration-200 ${iconClass} text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"></path>
        </svg>`;
    }

    /**
     * 渲染树节点行（IDEA 风格）
     */
    function renderTreeRow(node, depth = 0, parentId = null) {
        const nodeId = node.object_id || node.ref_id;
        const nodeKey = parentId ? `${parentId}:${nodeId}` : nodeId;
        const state = treeState.get(nodeKey) || { expanded: false, children: [], loaded: false };
        const hasChildren = node.has_children || (node.fields && node.fields.length > 0);
        const isExpanded = state.expanded;
        
        const indent = depth * 20;
        const shallowSize = node.shallow_size || 0;
        const retainedSize = node.retained_size || 0;
        
        // Determine display name
        let displayName = '';
        if (node.name) {
            // This is a field
            displayName = `<span class="text-blue-600 font-medium">${escapeHtml(node.name)}</span>`;
            if (node.ref_class) {
                displayName += ` = ${formatClassName(node.ref_class)}`;
            } else if (node.value !== undefined && node.value !== null) {
                displayName += ` = <span class="text-purple-600">${escapeHtml(String(node.value))}</span>`;
            }
        } else {
            // This is a top-level object
            displayName = formatClassName(node.class_name);
        }

        let html = `
            <div class="tree-row hover:bg-gray-50 border-b border-gray-100" data-node-id="${escapeHtml(nodeKey)}" data-depth="${depth}">
                <div class="flex items-center py-2 px-3 cursor-pointer" style="padding-left: ${indent + 12}px" onclick="HeapBiggestObjects.toggleNode('${escapeHtml(nodeKey)}', '${escapeHtml(nodeId)}')">
                    <span class="flex-shrink-0 mr-1">${renderExpandIcon(hasChildren, isExpanded)}</span>
                    <span class="flex-1 font-mono text-sm truncate" title="${escapeHtml(node.class_name || node.ref_class || '')}">${displayName}</span>
                    <span class="flex-shrink-0 w-24 text-right text-xs text-gray-500">${formatBytes(shallowSize)}</span>
                    <span class="flex-shrink-0 w-28 text-right text-sm font-semibold ${retainedSize > 1024*1024 ? 'text-red-600' : 'text-gray-700'}">${formatBytes(retainedSize)}</span>
                </div>
            </div>`;

        // Render children if expanded
        if (isExpanded && state.children && state.children.length > 0) {
            for (const child of state.children) {
                html += renderTreeRow(child, depth + 1, nodeKey);
            }
        } else if (isExpanded && !state.loaded) {
            // Show loading indicator
            html += `
                <div class="tree-row" style="padding-left: ${indent + 32}px">
                    <div class="flex items-center py-2 px-3 text-gray-400 text-sm">
                        <div class="animate-spin rounded-full h-4 w-4 border-2 border-gray-300 border-t-primary mr-2"></div>
                        Loading...
                    </div>
                </div>`;
        }

        return html;
    }

    /**
     * 渲染单个顶层对象
     */
    function renderTopLevelObject(obj, index) {
        const nodeId = obj.object_id;
        const state = treeState.get(nodeId) || { expanded: false, children: [], loaded: false };
        const hasChildren = (obj.fields && obj.fields.length > 0) || obj.has_children;
        const isExpanded = state.expanded;
        
        const retainedPercent = biggestObjects.length > 0 && biggestObjects[0].retained_size > 0 
            ? (obj.retained_size / biggestObjects[0].retained_size * 100) 
            : 0;

        let html = `
            <div class="biggest-object-item bg-white border border-gray-200 rounded-lg mb-2 overflow-hidden shadow-sm hover:shadow-md transition-shadow">
                <div class="tree-header flex items-center py-3 px-4 cursor-pointer hover:bg-gray-50" onclick="HeapBiggestObjects.toggleNode('${escapeHtml(nodeId)}', '${escapeHtml(nodeId)}')">
                    <span class="flex-shrink-0 w-6 text-center mr-2">
                        <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-primary text-white text-xs font-bold">${index + 1}</span>
                    </span>
                    <span class="flex-shrink-0 mr-2">${renderExpandIcon(hasChildren, isExpanded)}</span>
                    <div class="flex-1 min-w-0">
                        <div class="font-mono text-sm truncate" title="${escapeHtml(obj.class_name)}">${formatClassName(obj.class_name)}</div>
                        <div class="text-xs text-gray-400">ID: ${formatObjectId(obj.object_id)}</div>
                    </div>
                    <div class="flex-shrink-0 flex items-center gap-1 mr-2">
                        <button class="px-2 py-1 text-xs bg-blue-50 text-blue-600 rounded hover:bg-blue-100 transition-colors" onclick="event.stopPropagation(); HeapBiggestObjects.showGCRoots('${escapeHtml(nodeId)}')" title="Show GC Root Paths">
                            GC Roots
                        </button>
                        <button class="px-2 py-1 text-xs bg-purple-50 text-purple-600 rounded hover:bg-purple-100 transition-colors" onclick="event.stopPropagation(); HeapBiggestObjects.showRetainers('${escapeHtml(nodeId)}')" title="Show Retainers">
                            Retainers
                        </button>
                    </div>
                    <div class="flex-shrink-0 w-24 text-right px-2">
                        <div class="text-sm text-gray-600">${formatBytes(obj.shallow_size)}</div>
                        <div class="text-xs text-gray-400">Shallow</div>
                    </div>
                    <div class="flex-shrink-0 w-36 px-2">
                        <div class="flex items-center gap-2">
                            <div class="flex-1 h-1.5 bg-gray-100 rounded-full overflow-hidden">
                                <div class="h-full bg-gradient-to-r from-red-500 to-orange-500 rounded-full" style="width: ${Math.min(retainedPercent, 100)}%"></div>
                            </div>
                            <span class="text-sm font-bold text-red-600 w-20 text-right">${formatBytes(obj.retained_size)}</span>
                        </div>
                        <div class="text-xs text-gray-400 text-right">Retained</div>
                    </div>
                </div>`;

        // Render expanded children
        if (isExpanded) {
            html += '<div class="tree-children border-t border-gray-100 bg-gray-50">';
            
            // Header row
            html += `
                <div class="flex items-center py-1 px-4 bg-gray-100 border-b border-gray-200 text-xs text-gray-500 font-medium">
                    <span class="flex-1 pl-6">Item</span>
                    <span class="w-24 text-right">Shallow</span>
                    <span class="w-28 text-right">Retained</span>
                </div>`;

            if (state.children && state.children.length > 0) {
                for (const child of state.children) {
                    html += renderTreeRow(child, 1, nodeId);
                }
            } else if (!state.loaded) {
                html += `
                    <div class="flex items-center justify-center py-4 text-gray-400 text-sm">
                        <div class="animate-spin rounded-full h-4 w-4 border-2 border-gray-300 border-t-primary mr-2"></div>
                        Loading fields...
                    </div>`;
            } else {
                html += `
                    <div class="text-center py-4 text-gray-400 text-sm">
                        No fields available
                    </div>`;
            }
            
            html += '</div>';
        }

        html += '</div>';
        return html;
    }

    /**
     * 渲染统计摘要
     */
    function renderSummary() {
        const container = document.getElementById('biggestObjectsSummary');
        if (!container) return;
        
        const totalRetained = biggestObjects.reduce((sum, obj) => sum + (obj.retained_size || 0), 0);
        const totalShallow = biggestObjects.reduce((sum, obj) => sum + (obj.shallow_size || 0), 0);
        const uniqueClasses = new Set(biggestObjects.map(obj => obj.class_name)).size;
        
        container.innerHTML = `
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <div class="bg-gradient-to-br from-purple-50 to-purple-100 rounded-xl p-4 text-center border border-purple-200">
                    <div class="text-2xl font-bold text-purple-600">${biggestObjects.length}</div>
                    <div class="text-xs text-purple-500 mt-1">Total Objects</div>
                </div>
                <div class="bg-gradient-to-br from-red-50 to-red-100 rounded-xl p-4 text-center border border-red-200">
                    <div class="text-2xl font-bold text-red-600">${formatBytes(totalRetained)}</div>
                    <div class="text-xs text-red-500 mt-1">Total Retained</div>
                </div>
                <div class="bg-gradient-to-br from-blue-50 to-blue-100 rounded-xl p-4 text-center border border-blue-200">
                    <div class="text-2xl font-bold text-blue-600">${formatBytes(totalShallow)}</div>
                    <div class="text-xs text-blue-500 mt-1">Total Shallow</div>
                </div>
                <div class="bg-gradient-to-br from-green-50 to-green-100 rounded-xl p-4 text-center border border-green-200">
                    <div class="text-2xl font-bold text-green-600">${uniqueClasses}</div>
                    <div class="text-xs text-green-500 mt-1">Unique Classes</div>
                </div>
            </div>
            <div class="mt-3 text-xs text-gray-500 flex items-center gap-2">
                <span class="inline-block w-2 h-2 bg-green-500 rounded-full"></span>
                <span>Filtered: Basic types (byte[], Object[], ArrayList, HashMap, etc.) are hidden. Click to expand object fields.</span>
            </div>
        `;
    }

    /**
     * 渲染对象列表
     */
    function renderList() {
        const container = document.getElementById('biggestObjectsList');
        if (!container) return;
        
        if (filteredObjects.length === 0) {
            container.innerHTML = `
                <div class="text-center py-12 text-gray-500">
                    <svg class="w-16 h-16 mx-auto mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path>
                    </svg>
                    <p class="text-lg font-medium">No objects found</p>
                    <p class="text-sm mt-1">Try adjusting your search or filter criteria</p>
                </div>
            `;
            return;
        }
        
        container.innerHTML = filteredObjects.map((obj, i) => renderTopLevelObject(obj, i)).join('');
    }

    /**
     * 排序对象
     */
    function sortObjects() {
        filteredObjects.sort((a, b) => {
            let valA, valB;
            switch (currentSort.field) {
                case 'retained':
                    valA = a.retained_size || 0;
                    valB = b.retained_size || 0;
                    break;
                case 'shallow':
                    valA = a.shallow_size || 0;
                    valB = b.shallow_size || 0;
                    break;
                case 'class':
                    valA = a.class_name || '';
                    valB = b.class_name || '';
                    return currentSort.asc ? valA.localeCompare(valB) : valB.localeCompare(valA);
                case 'fields':
                    valA = (a.fields || []).length;
                    valB = (b.fields || []).length;
                    break;
                default:
                    valA = a.retained_size || 0;
                    valB = b.retained_size || 0;
            }
            return currentSort.asc ? valA - valB : valB - valA;
        });
    }

    /**
     * 更新排序按钮状态
     */
    function updateSortButtons() {
        document.querySelectorAll('.sort-btn').forEach(btn => {
            const field = btn.dataset.sort;
            btn.classList.remove('bg-primary', 'text-white');
            btn.classList.add('bg-gray-100', 'text-gray-700');
            
            if (field === currentSort.field) {
                btn.classList.remove('bg-gray-100', 'text-gray-700');
                btn.classList.add('bg-primary', 'text-white');
                
                // 更新箭头
                const arrow = btn.querySelector('.sort-arrow');
                if (arrow) {
                    arrow.textContent = currentSort.asc ? '↑' : '↓';
                }
            }
        });
    }

    /**
     * 加载对象字段（懒加载）
     * 优先使用 ReferenceGraph API 进行深度展开，回退到静态数据
     */
    async function loadObjectFields(objectId) {
        try {
            // 首先尝试使用 ReferenceGraph API（支持深度展开）
            const refGraphResponse = await fetch(`/api/refgraph/fields?id=${encodeURIComponent(objectId)}`);
            if (refGraphResponse.ok) {
                const fields = await refGraphResponse.json();
                if (Array.isArray(fields) && fields.length > 0) {
                    console.log(`[HeapBiggestObjects] Loaded ${fields.length} fields from ReferenceGraph for ${objectId}`);
                    return fields;
                }
            }
            
            // 回退到静态 API
            const response = await fetch(`/api/object-fields?id=${encodeURIComponent(objectId)}`);
            if (!response.ok) {
                console.warn(`Failed to load fields for ${objectId}: ${response.status}`);
                return [];
            }
            const fields = await response.json();
            return Array.isArray(fields) ? fields : [];
        } catch (error) {
            console.error(`Error loading fields for ${objectId}:`, error);
            return [];
        }
    }

    /**
     * 加载对象的 GC Root 路径
     */
    async function loadGCRootPaths(objectId, maxPaths = 3) {
        try {
            const response = await fetch(`/api/refgraph/gc-roots?id=${encodeURIComponent(objectId)}&max_paths=${maxPaths}`);
            if (!response.ok) {
                return [];
            }
            return await response.json();
        } catch (error) {
            console.error(`Error loading GC roots for ${objectId}:`, error);
            return [];
        }
    }

    /**
     * 加载对象的 Retainers（谁持有这个对象）
     */
    async function loadRetainers(objectId, maxRetainers = 20) {
        try {
            const response = await fetch(`/api/refgraph/retainers?id=${encodeURIComponent(objectId)}&max=${maxRetainers}`);
            if (!response.ok) {
                return [];
            }
            return await response.json();
        } catch (error) {
            console.error(`Error loading retainers for ${objectId}:`, error);
            return [];
        }
    }

    // ============================================
    // 公共方法
    // ============================================

    /**
     * 初始化模块
     */
    function init() {
        console.log('[HeapBiggestObjects] Initializing...');
        
        // 监听数据加载事件
        if (typeof HeapCore !== 'undefined') {
            HeapCore.on('dataLoaded', function(data) {
                loadBiggestObjects();
            });
        }
    }

    /**
     * 从 API 加载 Biggest Objects 数据
     */
    async function loadBiggestObjects(taskId) {
        const container = document.getElementById('biggestObjectsList');
        if (container) {
            container.innerHTML = `
                <div class="text-center py-12">
                    <div class="inline-block animate-spin rounded-full h-8 w-8 border-4 border-primary border-t-transparent"></div>
                    <p class="mt-4 text-gray-500">Loading biggest objects...</p>
                </div>
            `;
        }
        
        try {
            let url = '/api/biggest-objects';
            if (taskId) {
                url += `?task=${encodeURIComponent(taskId)}`;
            }
            
            const response = await fetch(url);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            
            const data = await response.json();
            biggestObjects = Array.isArray(data) ? data : [];
            filteredObjects = [...biggestObjects];
            
            // Initialize tree state for top-level objects
            treeState.clear();
            for (const obj of biggestObjects) {
                const hasChildren = (obj.fields && obj.fields.length > 0);
                treeState.set(obj.object_id, {
                    expanded: false,
                    children: obj.fields || [],
                    loaded: hasChildren,
                    hasChildren: hasChildren
                });
            }
            
            console.log('[HeapBiggestObjects] Loaded', biggestObjects.length, 'objects');
            
            sortObjects();
            renderSummary();
            renderList();
            updateSortButtons();
        } catch (error) {
            console.error('[HeapBiggestObjects] Failed to load data:', error);
            if (container) {
                container.innerHTML = `
                    <div class="text-center py-12 text-gray-500">
                        <svg class="w-16 h-16 mx-auto mb-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"></path>
                        </svg>
                        <p class="text-lg font-medium">Failed to load biggest objects</p>
                        <p class="text-sm mt-1">Please ensure retainer analysis is enabled</p>
                    </div>
                `;
            }
        }
    }

    /**
     * 渲染模块
     */
    function render() {
        renderSummary();
        renderList();
        updateSortButtons();
    }

    /**
     * 切换节点展开/折叠
     */
    async function toggleNode(nodeKey, objectId) {
        let state = treeState.get(nodeKey);
        
        if (!state) {
            state = { expanded: false, children: [], loaded: false };
            treeState.set(nodeKey, state);
        }

        if (state.expanded) {
            // Collapse
            state.expanded = false;
        } else {
            // Expand
            state.expanded = true;
            
            // Load children if not loaded
            if (!state.loaded) {
                // First check if this is a top-level object with fields already loaded
                const topObj = biggestObjects.find(o => o.object_id === objectId);
                if (topObj && topObj.fields && topObj.fields.length > 0) {
                    state.children = topObj.fields.map(f => ({
                        ...f,
                        has_children: f.ref_id ? true : false
                    }));
                    state.loaded = true;
                } else {
                    // Load from API
                    renderList(); // Show loading state
                    const fields = await loadObjectFields(objectId);
                    state.children = fields;
                    state.loaded = true;
                }
            }
        }

        treeState.set(nodeKey, state);
        renderList();
    }

    /**
     * 搜索过滤
     */
    function filter(searchTerm) {
        if (!searchTerm) {
            filteredObjects = [...biggestObjects];
        } else {
            const term = searchTerm.toLowerCase();
            filteredObjects = biggestObjects.filter(obj => 
                obj.class_name.toLowerCase().includes(term) ||
                (obj.object_id && obj.object_id.toLowerCase().includes(term))
            );
        }
        // Reset tree state for filtered objects
        treeState.clear();
        for (const obj of filteredObjects) {
            const hasChildren = (obj.fields && obj.fields.length > 0);
            treeState.set(obj.object_id, {
                expanded: false,
                children: obj.fields || [],
                loaded: hasChildren,
                hasChildren: hasChildren
            });
        }
        sortObjects();
        renderList();
    }

    /**
     * 排序
     */
    function sort(field) {
        if (currentSort.field === field) {
            currentSort.asc = !currentSort.asc;
        } else {
            currentSort.field = field;
            currentSort.asc = false;
        }
        sortObjects();
        renderList();
        updateSortButtons();
    }

    /**
     * 展开所有（只展开第一层）
     */
    function expandAll() {
        for (const obj of filteredObjects) {
            const state = treeState.get(obj.object_id);
            if (state) {
                state.expanded = true;
                if (!state.loaded && obj.fields) {
                    state.children = obj.fields.map(f => ({
                        ...f,
                        has_children: f.ref_id ? true : false
                    }));
                    state.loaded = true;
                }
            }
        }
        renderList();
    }

    /**
     * 折叠所有
     */
    function collapseAll() {
        for (const [key, state] of treeState) {
            state.expanded = false;
        }
        renderList();
    }

    /**
     * 刷新数据
     */
    function refresh(taskId) {
        treeState.clear();
        loadBiggestObjects(taskId);
    }

    /**
     * 显示 GC Root 路径弹窗
     */
    async function showGCRoots(objectId) {
        const modal = createModal('GC Root Paths', 'Loading...');
        document.body.appendChild(modal);
        
        try {
            const paths = await loadGCRootPaths(objectId, 5);
            
            if (!paths || paths.length === 0) {
                modal.querySelector('.modal-body').innerHTML = `
                    <div class="text-center py-8 text-gray-500">
                        <p>No GC root paths found for this object.</p>
                        <p class="text-sm mt-2">The object may be unreachable or the reference graph is not available.</p>
                    </div>
                `;
                return;
            }
            
            let html = `<div class="space-y-4">`;
            paths.forEach((path, idx) => {
                html += `
                    <div class="bg-gray-50 rounded-lg p-4 border border-gray-200">
                        <div class="flex items-center gap-2 mb-3">
                            <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-blue-500 text-white text-xs font-bold">${idx + 1}</span>
                            <span class="text-sm font-medium text-gray-700">Root Type: <span class="text-blue-600">${escapeHtml(path.root_type || 'Unknown')}</span></span>
                            <span class="text-xs text-gray-400">Depth: ${path.depth || path.path?.length || 0}</span>
                        </div>
                        <div class="space-y-1 ml-4 border-l-2 border-blue-200 pl-4">`;
                
                if (path.path && path.path.length > 0) {
                    path.path.forEach((node, nodeIdx) => {
                        const isLast = nodeIdx === path.path.length - 1;
                        html += `
                            <div class="flex items-center gap-2 py-1 ${isLast ? 'font-semibold' : ''}">
                                <span class="w-4 h-4 flex-shrink-0 ${isLast ? 'text-red-500' : 'text-gray-400'}">
                                    ${isLast ? '●' : '○'}
                                </span>
                                <span class="font-mono text-sm ${isLast ? 'text-red-600' : 'text-gray-700'}">${formatClassName(node.class_name || node.className, false)}</span>
                                ${node.field_name || node.fieldName ? `<span class="text-xs text-purple-500">.${escapeHtml(node.field_name || node.fieldName)}</span>` : ''}
                                ${node.size ? `<span class="text-xs text-gray-400 ml-auto">${formatBytes(node.size)}</span>` : ''}
                            </div>`;
                    });
                }
                
                html += `</div></div>`;
            });
            html += `</div>`;
            
            modal.querySelector('.modal-body').innerHTML = html;
        } catch (error) {
            modal.querySelector('.modal-body').innerHTML = `
                <div class="text-center py-8 text-red-500">
                    <p>Failed to load GC root paths</p>
                    <p class="text-sm mt-2">${escapeHtml(error.message)}</p>
                </div>
            `;
        }
    }

    /**
     * 显示 Retainers 弹窗
     */
    async function showRetainers(objectId) {
        const modal = createModal('Object Retainers', 'Loading...');
        document.body.appendChild(modal);
        
        try {
            const retainers = await loadRetainers(objectId, 30);
            
            if (!retainers || retainers.length === 0) {
                modal.querySelector('.modal-body').innerHTML = `
                    <div class="text-center py-8 text-gray-500">
                        <p>No retainers found for this object.</p>
                        <p class="text-sm mt-2">This object may be a GC root or the reference graph is not available.</p>
                    </div>
                `;
                return;
            }
            
            let html = `
                <div class="text-sm text-gray-500 mb-4">
                    Found ${retainers.length} object(s) that retain this object:
                </div>
                <div class="overflow-x-auto">
                    <table class="w-full text-sm">
                        <thead class="bg-gray-100">
                            <tr>
                                <th class="px-3 py-2 text-left text-gray-600">#</th>
                                <th class="px-3 py-2 text-left text-gray-600">Class</th>
                                <th class="px-3 py-2 text-left text-gray-600">Field</th>
                                <th class="px-3 py-2 text-right text-gray-600">Shallow</th>
                                <th class="px-3 py-2 text-right text-gray-600">Retained</th>
                            </tr>
                        </thead>
                        <tbody>`;
            
            retainers.forEach((r, idx) => {
                html += `
                    <tr class="border-b border-gray-100 hover:bg-gray-50">
                        <td class="px-3 py-2 text-gray-400">${idx + 1}</td>
                        <td class="px-3 py-2 font-mono">${formatClassName(r.class_name)}</td>
                        <td class="px-3 py-2 text-purple-600">${escapeHtml(r.field_name || '-')}</td>
                        <td class="px-3 py-2 text-right text-gray-600">${formatBytes(r.shallow_size || 0)}</td>
                        <td class="px-3 py-2 text-right font-semibold text-red-600">${formatBytes(r.retained_size || 0)}</td>
                    </tr>`;
            });
            
            html += `</tbody></table></div>`;
            
            modal.querySelector('.modal-body').innerHTML = html;
        } catch (error) {
            modal.querySelector('.modal-body').innerHTML = `
                <div class="text-center py-8 text-red-500">
                    <p>Failed to load retainers</p>
                    <p class="text-sm mt-2">${escapeHtml(error.message)}</p>
                </div>
            `;
        }
    }

    /**
     * 创建模态框
     */
    function createModal(title, content) {
        const modal = document.createElement('div');
        modal.className = 'fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-50';
        modal.onclick = (e) => {
            if (e.target === modal) {
                modal.remove();
            }
        };
        
        modal.innerHTML = `
            <div class="bg-white rounded-xl shadow-2xl max-w-3xl w-full mx-4 max-h-[80vh] flex flex-col" onclick="event.stopPropagation()">
                <div class="flex items-center justify-between px-6 py-4 border-b border-gray-200">
                    <h3 class="text-lg font-semibold text-gray-800">${escapeHtml(title)}</h3>
                    <button class="text-gray-400 hover:text-gray-600 transition-colors" onclick="this.closest('.fixed').remove()">
                        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
                        </svg>
                    </button>
                </div>
                <div class="modal-body flex-1 overflow-auto px-6 py-4">
                    ${content}
                </div>
            </div>
        `;
        
        return modal;
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        loadBiggestObjects,
        toggleNode,
        filter,
        sort,
        expandAll,
        collapseAll,
        refresh,
        showGCRoots,
        showRetainers
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('biggestObjects', module);
    }

    return module;
})();

// 导出到全局
window.HeapBiggestObjects = HeapBiggestObjects;
