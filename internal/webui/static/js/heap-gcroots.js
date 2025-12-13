/**
 * Heap GC Roots Module
 * GC Roots 分析模块：负责 GC Root 的展示和分析
 * 
 * 职责：
 * - 构建 GC Roots 数据结构
 * - 渲染 GC Roots 表格
 * - 处理过滤和展开/折叠
 * - 支持递归展开查看引用链
 */

const HeapGCRoots = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let gcRootsData = [];
    let expandedNodes = new Set();
    let classDataMap = new Map(); // 类名 -> 类数据的映射

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
        const classData = HeapCore.getState('classData') || [];
        
        // 构建类名映射
        classDataMap.clear();
        classData.forEach(cls => {
            const name = cls.class_name || cls.name || '';
            if (name) {
                classDataMap.set(name, cls);
            }
        });
        
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
                            retainedClasses: new Set(),
                            // 保存完整路径用于展开
                            paths: []
                        });
                    }
                    
                    const root = rootMap.get(rootKey);
                    root.retainedClasses.add(className);
                    root.paths.push({ targetClass: className, path: path.path });
                    
                    // 累加 retained size
                    const classInfo = classData.find(c => (c.class_name || c.name) === className);
                    if (classInfo) {
                        root.retained_size += classInfo.retained_size || classInfo.total_size || classInfo.size || 0;
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
                                retainedClasses: new Set(),
                                paths: []
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
     * 获取类的 retainers（用于递归展开）
     */
    function getRetainersForClass(className) {
        const classInfo = classDataMap.get(className);
        if (classInfo && classInfo.retainers) {
            return classInfo.retainers;
        }
        
        // 尝试模糊匹配
        const shortName = Utils.getShortClassName(className);
        for (const [name, cls] of classDataMap) {
            if (Utils.getShortClassName(name) === shortName && cls.retainers) {
                return cls.retainers;
            }
        }
        
        return [];
    }

    /**
     * 渲染递归展开的引用链
     */
    function renderReferenceChain(className, nodeId, level = 0) {
        const retainers = getRetainersForClass(className);
        const maxLevel = 5;
        
        if (level >= maxLevel) {
            return `<div class="chain-max-depth" style="padding-left: ${(level + 1) * 20}px;">
                ⚠️ 已达到最大展开深度
            </div>`;
        }
        
        if (retainers.length === 0) {
            return `<div class="chain-no-data" style="padding-left: ${(level + 1) * 20}px; color: #808080;">
                没有更多 retainer 数据
            </div>`;
        }
        
        // 按 retained_size 排序
        const sortedRetainers = [...retainers].sort((a, b) => 
            (b.retained_size || 0) - (a.retained_size || 0)
        );
        
        return sortedRetainers.slice(0, 10).map((retainer, idx) => {
            const retainerClass = retainer.retainer_class || retainer.class_name || 'Unknown';
            const fieldName = retainer.field_name || '';
            const retainedSize = retainer.retained_size || 0;
            const percentage = retainer.percentage || 0;
            const childNodeId = `${nodeId}-r${idx}`;
            const isExpanded = expandedNodes.has(childNodeId);
            const hasChildren = getRetainersForClass(retainerClass).length > 0;
            const isBusinessClass = checkIsBusinessClass(retainerClass);
            
            return `
                <div class="chain-node" style="padding-left: ${(level + 1) * 20}px;" data-node-id="${childNodeId}">
                    <div class="chain-row ${hasChildren ? 'expandable' : ''} ${isBusinessClass ? 'business-class' : ''}"
                         onclick="HeapGCRoots.toggleChainNode('${childNodeId}', '${Utils.escapeHtml(retainerClass).replace(/'/g, "\\'")}', ${level + 1}); event.stopPropagation();">
                        <span class="chain-expand">${hasChildren ? (isExpanded ? '▼' : '▶') : '─'}</span>
                        <span class="chain-icon">${isBusinessClass ? '🎯' : '📦'}</span>
                        <span class="chain-class ${isBusinessClass ? 'highlight' : ''}" title="${Utils.escapeHtml(retainerClass)}">
                            ${Utils.escapeHtml(Utils.getShortClassName(retainerClass))}
                        </span>
                        ${fieldName ? `<span class="chain-field">.${Utils.escapeHtml(fieldName)}</span>` : ''}
                        <span class="chain-stats">
                            <span class="chain-percentage">${percentage.toFixed(1)}%</span>
                            <span class="chain-size">${Utils.formatBytes(retainedSize)}</span>
                        </span>
                        ${isBusinessClass ? '<span class="business-badge">业务类</span>' : ''}
                    </div>
                    <div id="${childNodeId}-children" class="chain-children" style="display: ${isExpanded ? 'block' : 'none'};">
                        ${isExpanded ? renderReferenceChain(retainerClass, childNodeId, level + 1) : ''}
                    </div>
                </div>
            `;
        }).join('') + (sortedRetainers.length > 10 ? 
            `<div class="chain-more" style="padding-left: ${(level + 1) * 20}px;">还有 ${sortedRetainers.length - 10} 个...</div>` : '');
    }

    /**
     * 检查是否是业务类
     */
    function checkIsBusinessClass(className) {
        if (!className) return false;
        
        // JDK 类
        if (className.startsWith('java.') || className.startsWith('javax.') ||
            className.startsWith('sun.') || className.startsWith('com.sun.') ||
            className.startsWith('jdk.')) {
            return false;
        }
        
        // 数组类型
        if (className.includes('[]')) return false;
        
        // 框架内部类
        const frameworkPrefixes = [
            'org.springframework.aop.', 'org.springframework.beans.factory.support.',
            'io.netty.buffer.Pool', 'io.netty.util.internal.',
            'com.google.common.collect.', 'com.google.common.cache.',
            'org.slf4j.', 'ch.qos.logback.',
            'com.fasterxml.jackson.core.', 'com.fasterxml.jackson.databind.cfg.'
        ];
        
        for (const prefix of frameworkPrefixes) {
            if (className.startsWith(prefix)) return false;
        }
        
        return true;
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
            const isExpanded = expandedNodes.has(`gc-root-${i}`);
            
            return `
                <tr class="gc-root-row" onclick="HeapGCRoots.toggleRow(${i})">
                    <td><button class="expand-btn" id="gc-expand-${i}">${isExpanded ? '▼' : '▶'}</button></td>
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
                <tr id="gc-children-${i}" class="gc-root-children" style="display: ${isExpanded ? 'table-row' : 'none'};">
                    <td colspan="4">
                        <div class="gc-root-detail">
                            <div class="gc-root-detail-header">
                                <span>🔗 Retains ${root.retainedClasses.length} class(es)</span>
                                <span class="detail-hint">点击类名可递归展开查看引用链</span>
                            </div>
                            <div class="gc-root-retained-classes">
                                ${root.retainedClasses.slice(0, 15).map((cls, clsIdx) => {
                                    const nodeId = `gc-${i}-cls-${clsIdx}`;
                                    const isClsExpanded = expandedNodes.has(nodeId);
                                    const hasRetainers = getRetainersForClass(cls).length > 0;
                                    const isBusinessClass = checkIsBusinessClass(cls);
                                    
                                    return `
                                        <div class="retained-class-item">
                                            <div class="retained-class-header ${hasRetainers ? 'expandable' : ''} ${isBusinessClass ? 'business-class' : ''}"
                                                 onclick="HeapGCRoots.toggleClassNode('${nodeId}', '${Utils.escapeHtml(cls).replace(/'/g, "\\'")}'); event.stopPropagation();">
                                                <span class="expand-indicator">${hasRetainers ? (isClsExpanded ? '▼' : '▶') : '─'}</span>
                                                <span class="class-icon">${isBusinessClass ? '🎯' : '📦'}</span>
                                                <span class="class-name ${isBusinessClass ? 'highlight' : ''}" title="${Utils.escapeHtml(cls)}">
                                                    ${Utils.escapeHtml(Utils.getShortClassName(cls))}
                                                </span>
                                                ${isBusinessClass ? '<span class="business-badge">业务类</span>' : ''}
                                                <button class="search-btn" onclick="HeapHistogram.searchClass('${Utils.escapeHtml(cls).replace(/'/g, "\\'")}'); event.stopPropagation();">
                                                    🔍
                                                </button>
                                            </div>
                                            <div id="${nodeId}-children" class="retained-class-children" style="display: ${isClsExpanded ? 'block' : 'none'};">
                                                ${isClsExpanded ? renderReferenceChain(cls, nodeId, 0) : ''}
                                            </div>
                                        </div>
                                    `;
                                }).join('')}
                                ${root.retainedClasses.length > 15 ? 
                                    `<div class="more-classes">... 还有 ${root.retainedClasses.length - 15} 个类</div>` : ''}
                            </div>
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
            expandedNodes.clear();
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
                r.type.toLowerCase().includes(searchTerm) ||
                r.retainedClasses.some(c => c.toLowerCase().includes(searchTerm))
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
        const nodeId = `gc-root-${idx}`;
        const childrenRow = document.getElementById(`gc-children-${idx}`);
        const expandBtn = document.getElementById(`gc-expand-${idx}`);
        
        if (childrenRow) {
            const isVisible = childrenRow.style.display !== 'none';
            
            if (isVisible) {
                expandedNodes.delete(nodeId);
                childrenRow.style.display = 'none';
                if (expandBtn) expandBtn.textContent = '▶';
            } else {
                expandedNodes.add(nodeId);
                childrenRow.style.display = 'table-row';
                if (expandBtn) expandBtn.textContent = '▼';
            }
        }
    }

    /**
     * 切换类节点展开/折叠
     */
    function toggleClassNode(nodeId, className) {
        const childrenContainer = document.getElementById(`${nodeId}-children`);
        const nodeElement = document.querySelector(`[onclick*="${nodeId}"]`);
        const indicator = nodeElement?.querySelector('.expand-indicator');
        
        if (!childrenContainer) return;
        
        const isHidden = childrenContainer.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(nodeId);
            // 动态渲染子节点
            if (childrenContainer.innerHTML.trim() === '') {
                childrenContainer.innerHTML = renderReferenceChain(className, nodeId, 0);
            }
            childrenContainer.style.display = 'block';
            if (indicator) indicator.textContent = '▼';
        } else {
            expandedNodes.delete(nodeId);
            childrenContainer.style.display = 'none';
            if (indicator) indicator.textContent = '▶';
        }
    }

    /**
     * 切换引用链节点展开/折叠
     */
    function toggleChainNode(nodeId, className, level) {
        const childrenContainer = document.getElementById(`${nodeId}-children`);
        const nodeElement = document.querySelector(`[data-node-id="${nodeId}"]`);
        const indicator = nodeElement?.querySelector('.chain-expand');
        
        if (!childrenContainer) return;
        
        const retainers = getRetainersForClass(className);
        if (retainers.length === 0) {
            HeapCore.showNotification(`${Utils.getShortClassName(className)} 没有更多 retainer 数据`, 'info');
            return;
        }
        
        const isHidden = childrenContainer.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(nodeId);
            // 动态渲染子节点
            if (childrenContainer.innerHTML.trim() === '') {
                childrenContainer.innerHTML = renderReferenceChain(className, nodeId, level);
            }
            childrenContainer.style.display = 'block';
            if (indicator) indicator.textContent = '▼';
        } else {
            expandedNodes.delete(nodeId);
            childrenContainer.style.display = 'none';
            if (indicator) indicator.textContent = '▶';
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
        toggleClassNode,
        toggleChainNode,
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
