/**
 * Heap GC Roots Module
 * GC Roots 分析模块：负责 GC Root 的展示和分析
 * 
 * 职责：
 * - 构建 GC Roots 数据结构
 * - 渲染 GC Roots 表格
 * - 处理过滤和展开/折叠
 */

const HeapGCRoots = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let gcRootsData = [];

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 从类数据和路径数据构建 GC Roots
     * @returns {Array} GC Roots 数据数组
     */
    function buildGCRootsData() {
        const gcRootPaths = HeapCore.getState('gcRootPaths');
        const referenceGraphs = HeapCore.getState('referenceGraphs');
        const classData = HeapCore.getState('classData');
        
        const rootMap = new Map();

        // 从 gcRootPaths 提取 GC roots
        for (const [className, paths] of Object.entries(gcRootPaths)) {
            for (const path of paths) {
                if (path.path && path.path.length > 0) {
                    const rootNode = path.path[0];
                    const rootKey = `${path.root_type}:${rootNode.class_name}`;
                    
                    if (!rootMap.has(rootKey)) {
                        rootMap.set(rootKey, {
                            type: path.root_type || 'Unknown',
                            class_name: rootNode.class_name,
                            shallow_size: rootNode.size || 0,
                            retained_size: 0,
                            children: [],
                            retainedClasses: new Set()
                        });
                    }
                    
                    const root = rootMap.get(rootKey);
                    root.retainedClasses.add(className);
                    
                    // 累加 retained size
                    const classInfo = classData.find(c => c.name === className);
                    if (classInfo) {
                        root.retained_size += classInfo.retained_size || classInfo.size || 0;
                    }
                }
            }
        }

        // 从 reference graphs 提取 GC root 信息
        for (const [className, graph] of Object.entries(referenceGraphs)) {
            if (graph && graph.nodes) {
                for (const node of graph.nodes) {
                    if (node.is_gc_root) {
                        const rootKey = `${node.gc_root_type || 'Root'}:${node.class_name}`;
                        if (!rootMap.has(rootKey)) {
                            rootMap.set(rootKey, {
                                type: node.gc_root_type || 'Root',
                                class_name: node.class_name,
                                shallow_size: node.size || 0,
                                retained_size: node.retained_size || 0,
                                children: [],
                                retainedClasses: new Set()
                            });
                        }
                        const root = rootMap.get(rootKey);
                        root.retainedClasses.add(className);
                    }
                }
            }
        }

        // 转换为数组并排序
        return Array.from(rootMap.values())
            .map(r => ({
                ...r,
                retainedClasses: Array.from(r.retainedClasses)
            }))
            .sort((a, b) => b.retained_size - a.retained_size);
    }

    /**
     * 渲染 GC Roots 表格
     * @param {Array} roots - GC Roots 数据
     */
    function renderTable(roots) {
        const tbody = document.getElementById('gcRootsTableBody');
        if (!tbody) return;

        if (roots.length === 0) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="4" class="no-data-message" style="text-align: center; padding: 40px;">
                        <div class="icon">🌳</div>
                        <div>No GC Roots data available</div>
                        <div style="font-size: 12px; color: #808080; margin-top: 8px;">
                            GC Root analysis requires heap dump with dominator tree data
                        </div>
                    </td>
                </tr>
            `;
            return;
        }

        const maxRetained = Math.max(...roots.map(r => r.retained_size), 1);

        tbody.innerHTML = roots.map((root, i) => {
            const retainedBarWidth = maxRetained > 0 ? (root.retained_size / maxRetained) * 100 : 0;
            
            return `
                <tr class="gc-root-row" onclick="HeapGCRoots.toggleRow(${i})">
                    <td><button class="expand-btn" id="gc-expand-${i}">▶</button></td>
                    <td>
                        <span class="gc-root-type">${Utils.escapeHtml(root.type)}</span>
                        <span class="gc-root-name">${Utils.escapeHtml(Utils.getShortClassName(root.class_name))}</span>
                    </td>
                    <td>${Utils.formatBytes(root.shallow_size)}</td>
                    <td class="size-cell retained-cell">
                        <div class="size-bar-bg" style="width: ${retainedBarWidth}%"></div>
                        <span class="size-value">${Utils.formatBytes(root.retained_size)}</span>
                    </td>
                </tr>
                <tr id="gc-children-${i}" class="gc-root-children" style="display: none;">
                    <td colspan="4">
                        <div style="padding: 10px 20px;">
                            <div style="color: #808080; font-size: 12px; margin-bottom: 8px;">
                                Retains ${root.retainedClasses.length} class(es):
                            </div>
                            ${root.retainedClasses.slice(0, 10).map(cls => `
                                <div style="padding: 4px 0; display: flex; align-items: center;">
                                    <span style="color: #cc7832; margin-right: 8px;">📦</span>
                                    <span style="color: #ffc66d; cursor: pointer;" onclick="HeapHistogram.searchClass('${Utils.escapeHtml(cls).replace(/'/g, "\\'")}'); event.stopPropagation();">
                                        ${Utils.escapeHtml(Utils.getShortClassName(cls))}
                                    </span>
                                </div>
                            `).join('')}
                            ${root.retainedClasses.length > 10 ? `<div style="color: #808080; padding: 4px 0;">... and ${root.retainedClasses.length - 10} more</div>` : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }

    /**
     * 更新汇总信息
     * @param {Array} roots - GC Roots 数据
     */
    function updateSummary(roots) {
        const summaryCount = document.getElementById('gcRootsTotalCount');
        const summarySize = document.getElementById('gcRootsRetainedSize');
        
        const totalRoots = roots.length;
        const totalRetained = roots.reduce((sum, r) => sum + (r.retained_size || 0), 0);
        
        if (summaryCount) summaryCount.textContent = Utils.formatNumber(totalRoots);
        if (summarySize) summarySize.textContent = Utils.formatBytes(totalRetained);
    }

    // ============================================
    // 公共方法
    // ============================================
    
    /**
     * 初始化模块
     */
    function init() {
        // 监听数据加载事件
        HeapCore.on('dataLoaded', function() {
            render();
        });

        // 监听 retainer 数据更新
        HeapCore.on('retainerDataUpdated', function() {
            render();
        });
    }

    /**
     * 渲染 GC Roots
     */
    function render() {
        gcRootsData = buildGCRootsData();
        updateSummary(gcRootsData);
        renderTable(gcRootsData);
    }

    /**
     * 过滤 GC Roots
     */
    function filter() {
        const searchTerm = document.getElementById('gcRootsSearch')?.value?.toLowerCase() || '';
        const typeFilter = document.getElementById('gcRootsTypeFilter')?.value || '';
        
        let filtered = gcRootsData;
        
        if (searchTerm) {
            filtered = filtered.filter(r => 
                r.class_name.toLowerCase().includes(searchTerm) ||
                r.type.toLowerCase().includes(searchTerm)
            );
        }
        
        if (typeFilter) {
            filtered = filtered.filter(r => r.type.includes(typeFilter));
        }
        
        renderTable(filtered);
    }

    /**
     * 切换行展开/折叠
     * @param {number} idx - 行索引
     */
    function toggleRow(idx) {
        const childrenRow = document.getElementById(`gc-children-${idx}`);
        const expandBtn = document.getElementById(`gc-expand-${idx}`);
        
        if (childrenRow) {
            const isVisible = childrenRow.style.display !== 'none';
            childrenRow.style.display = isVisible ? 'none' : 'table-row';
            if (expandBtn) expandBtn.textContent = isVisible ? '▶' : '▼';
        }
    }

    /**
     * 获取 GC Roots 数据
     * @returns {Array} GC Roots 数据
     */
    function getData() {
        return gcRootsData;
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        filter,
        toggleRow,
        getData
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('gcroots', module);
    }

    return module;
})();

// 导出到全局
window.HeapGCRoots = HeapGCRoots;
