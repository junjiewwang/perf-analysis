/**
 * Heap Merged Paths Module
 * Merged Paths 分析模块：IDEA 风格的合并路径展示
 * 
 * 职责：
 * - 展示 Top 内存占用类列表
 * - 用户展开类卡片时，通过 /api/refgraph/class-retainers 按需加载 retainer 数据
 * - 渲染类似 IDEA Memory Profiler 的树视图
 * - 处理展开/折叠操作
 * - 支持递归展开 retainer（查看 retainer 的 retainer）
 * 
 * 设计原则：
 * - 遵循 "serve-time read" 原则，retainer 数据通过 HeapQueryEngine on-demand 查询
 * - 懒加载：只在用户展开时请求数据，避免首屏 N 次并发请求
 */

const HeapMergedPaths = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let expandedNodes = new Set();
    let loadedRetainers = new Map(); // className -> retainer data (from API)
    let classDataMap = new Map(); // 类名 -> 类数据的映射
    let currentTaskId = null;

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 获取当前 taskId
     */
    function getCurrentTaskId() {
        if (currentTaskId) return currentTaskId;
        if (typeof App !== 'undefined' && App.getCurrentTask) {
            const taskId = App.getCurrentTask();
            if (taskId) return taskId;
        }
        const urlParams = new URLSearchParams(window.location.search);
        const urlTaskId = urlParams.get('task');
        if (urlTaskId) return urlTaskId;
        if (window.currentTaskId) return window.currentTaskId;
        return null;
    }

    /**
     * 获取 top 类列表（按内存大小排序）
     * 不再依赖 retainers 字段过滤，直接显示所有 top classes
     * @returns {Array} top classes 列表
     */
    function getTopClasses() {
        const classData = HeapCore.getState('classData') || [];
        
        // 构建类名映射
        classDataMap.clear();
        classData.forEach(cls => {
            const name = cls.class_name || cls.name || '';
            if (name) {
                classDataMap.set(name, cls);
            }
        });
        
        // 按 retained_size（或 size）排序，返回 top 30
        return [...classData]
            .filter(cls => (cls.class_name || cls.name || ''))
            .sort((a, b) => (b.retained_size || b.size || 0) - (a.retained_size || a.size || 0))
            .slice(0, 30);
    }

    /**
     * 通过 API 按需获取某个类的 retainers
     * @param {string} className - 类名
     * @returns {Promise<Array>} retainers 数组
     */
    async function fetchRetainersForClass(className) {
        // 先从缓存查找
        if (loadedRetainers.has(className)) {
            return loadedRetainers.get(className);
        }
        
        const taskId = getCurrentTaskId();
        if (!taskId) {
            console.warn('[HeapMergedPaths] No taskId available for API call');
            return [];
        }

        try {
            const retainers = await API.getClassRetainers(taskId, className, 20);
            loadedRetainers.set(className, retainers || []);
            return retainers || [];
        } catch (error) {
            console.error(`[HeapMergedPaths] Failed to fetch retainers for ${className}:`, error);
            return [];
        }
    }

    /**
     * 检查是否是业务类
     */
    function checkIsBusinessClass(className) {
        if (!className) return false;
        
        if (className.startsWith('java.') || className.startsWith('javax.') ||
            className.startsWith('sun.') || className.startsWith('com.sun.') ||
            className.startsWith('jdk.')) {
            return false;
        }
        
        if (className.includes('[]')) return false;
        
        const frameworkPrefixes = [
            'org.springframework.aop.', 'org.springframework.beans.factory.support.',
            'io.netty.buffer.Pool', 'io.netty.util.internal.', 'io.netty.util.Recycler',
            'com.google.common.collect.', 'com.google.common.cache.',
            'org.slf4j.', 'ch.qos.logback.',
            'com.fasterxml.jackson.core.', 'com.fasterxml.jackson.databind.cfg.',
            'net.bytebuddy.', 'io.opentelemetry.javaagent.'
        ];
        
        for (const prefix of frameworkPrefixes) {
            if (className.startsWith(prefix)) return false;
        }
        
        return true;
    }

    /**
     * 判断是否是 GC Root 类
     */
    function isGCRootClass(className) {
        const gcRootPatterns = [
            'java.lang.Thread',
            'java.lang.Class',
            'java.lang.ClassLoader',
            'JNI Global',
            'System Class',
            'Thread Block',
            'Busy Monitor',
            'Native Stack',
            'Finalizer'
        ];
        return gcRootPatterns.some(pattern => className.includes(pattern));
    }

    /**
     * 渲染单个 retainer 节点（支持递归展开）
     */
    function renderRetainerNode(retainer, parentId, index, level = 0) {
        const retainerClass = retainer.retainer_class || retainer.class_name || 'Unknown';
        const fieldName = retainer.field_name || '';
        const retainedSize = retainer.retained_size || 0;
        const retainedCount = retainer.retained_count || 0;
        const percentage = retainer.percentage || 0;
        
        const shortName = Utils.getShortClassName(retainerClass);
        const nodeId = `${parentId}-r${index}`;
        const isExpanded = expandedNodes.has(nodeId);
        const isGCRoot = isGCRootClass(retainerClass);
        const isBusinessClass = checkIsBusinessClass(retainerClass);
        
        // 计算缩进
        const indent = level * 20;
        
        // 检查是否可以继续展开（递归查 retainer 的 retainer）
        const canExpand = level < 5 && !isGCRoot;
        
        let html = `
            <div class="retainer-node level-${level}" data-node-id="${nodeId}" data-class="${Utils.escapeHtml(retainerClass)}" style="padding-left: ${indent}px;">
                <div class="retainer-row ${canExpand ? 'expandable' : ''} ${isGCRoot ? 'gc-root' : ''} ${isBusinessClass ? 'business-class' : ''}" 
                     onclick="HeapMergedPaths.toggleRetainerNode('${nodeId}', '${Utils.escapeHtml(retainerClass).replace(/'/g, "\\'")}', ${level})">
                    <span class="expand-indicator">${canExpand ? (isExpanded ? '▼' : '▶') : (isGCRoot ? '🌳' : '─')}</span>
                    <span class="retainer-icon">${isGCRoot ? '🌳' : (isBusinessClass ? '🎯' : '📦')}</span>
                    <span class="retainer-class ${isBusinessClass ? 'highlight' : ''}" title="${Utils.escapeHtml(retainerClass)}">${Utils.escapeHtml(shortName)}</span>
                    ${fieldName ? `<span class="retainer-field">.${Utils.escapeHtml(fieldName)}</span>` : ''}
                    <span class="retainer-stats">
                        <span class="stat-percentage" title="占比">${percentage.toFixed(1)}%</span>
                        <span class="stat-size" title="保留大小">${Utils.formatBytes(retainedSize)}</span>
                        <span class="stat-count" title="保留对象数">×${retainedCount.toLocaleString()}</span>
                    </span>
                    ${isGCRoot ? '<span class="gc-root-badge">GC Root</span>' : ''}
                    ${isBusinessClass ? '<span class="business-badge">业务类</span>' : ''}
                </div>
                <div id="${nodeId}-children" class="retainer-children" style="display: ${isExpanded ? 'block' : 'none'};">
                </div>
            </div>
        `;
        
        return html;
    }

    /**
     * 渲染 retainers 树
     */
    function renderRetainersTree(retainers, cardId) {
        if (!retainers || retainers.length === 0) {
            return '<div class="no-retainers">没有 retainer 数据</div>';
        }

        // 按 retained_size 排序
        const sortedRetainers = [...retainers].sort((a, b) => 
            (b.retained_size || 0) - (a.retained_size || 0)
        );

        let html = '<div class="retainers-tree">';
        
        sortedRetainers.forEach((retainer, index) => {
            html += renderRetainerNode(retainer, cardId, index, 0);
        });
        
        html += '</div>';
        return html;
    }

    /**
     * 渲染单个类的卡片
     */
    function renderClassCard(classInfo, index) {
        const cardId = `merged-class-${index}`;
        const isExpanded = expandedNodes.has(cardId);
        const className = classInfo.class_name || classInfo.name || '';
        const shortName = Utils.getShortClassName(className);
        const isBusinessClass = checkIsBusinessClass(className);
        
        return `
            <div class="merged-class-card ${isBusinessClass ? 'business-class-card' : ''}" data-class-name="${Utils.escapeHtml(className)}">
                <div class="merged-class-header" onclick="HeapMergedPaths.toggleClassCard('${cardId}', '${Utils.escapeHtml(className).replace(/'/g, "\\'")}')">
                    <span class="expand-indicator">${isExpanded ? '▼' : '▶'}</span>
                    <span class="class-icon">${isBusinessClass ? '🎯' : '📦'}</span>
                    <span class="class-name" title="${Utils.escapeHtml(className)}">${Utils.escapeHtml(shortName)}</span>
                    <span class="class-stats">
                        <span class="stat-item" title="实例数量">
                            📊 ${(classInfo.instance_count || classInfo.instanceCount || classInfo.count || 0).toLocaleString()} instances
                        </span>
                        <span class="stat-item" title="内存占用">
                            💾 ${Utils.formatBytes(classInfo.total_size || classInfo.size || 0)}
                        </span>
                        ${classInfo.retained_size ? `
                            <span class="stat-item" title="Retained Size">
                                🔒 ${Utils.formatBytes(classInfo.retained_size)}
                            </span>
                        ` : ''}
                    </span>
                </div>
                <div id="${cardId}" class="merged-class-content" style="display: ${isExpanded ? 'block' : 'none'};">
                    <div id="${cardId}-retainers" class="retainers-content">
                        ${isExpanded ? '<div class="loading-retainers">Loading retainers...</div>' : ''}
                    </div>
                </div>
            </div>
        `;
    }

    /**
     * 加载并渲染某个类的 retainers（展开时调用）
     */
    async function loadAndRenderRetainers(cardId, className) {
        const container = document.getElementById(`${cardId}-retainers`);
        if (!container) return;

        container.innerHTML = '<div class="loading-retainers">⏳ Loading retainers...</div>';

        const retainers = await fetchRetainersForClass(className);

        if (retainers.length === 0) {
            container.innerHTML = `
                <div class="no-retainers-message">
                    <span>📭</span> No retainer data found for this class.
                    <div class="hint">This class may be a GC root or have no incoming references.</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="retainers-header">
                <span class="header-title">📍 Retained by (谁持有这个类的实例)</span>
                <span class="header-hint">💡 点击类名展开查看持有者的持有者</span>
            </div>
            ${renderRetainersTree(retainers, cardId)}
        `;
    }

    /**
     * 渲染所有类的 Merged Paths
     */
    function renderAllMergedPaths() {
        const container = document.getElementById('mergedPathsContainer');
        if (!container) return;

        const topClasses = getTopClasses();
        
        console.log('[HeapMergedPaths] Top classes for merged paths:', topClasses.length);
        
        if (topClasses.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">🔀</div>
                    <div class="title">没有找到类数据</div>
                    <div class="hint">
                        请确保 Heap Dump 分析已完成且加载了数据。
                    </div>
                </div>
            `;
            return;
        }

        let html = `
            <div class="merged-paths-summary">
                <div class="summary-stat">
                    <span class="stat-value">${topClasses.length}</span>
                    <span class="stat-label">Top Memory Classes</span>
                </div>
            </div>
            <div class="merged-paths-toolbar">
                <button class="toolbar-btn" onclick="HeapMergedPaths.expandAll()">
                    📂 Expand All
                </button>
                <button class="toolbar-btn" onclick="HeapMergedPaths.collapseAll()">
                    📁 Collapse All
                </button>
            </div>
            <div class="merged-paths-tips">
                <span>💡 展示内存占用大类被哪些类持有 (Retained by)</span>
                <span>🔍 点击类名展开，按需加载持有者数据</span>
                <span>📊 按内存大小排序</span>
            </div>
            <div class="merged-classes-list">
        `;

        // 渲染每个类的卡片
        topClasses.forEach((cls, index) => {
            html += renderClassCard(cls, index);
        });

        html += '</div>';
        container.innerHTML = html;
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
            loadedRetainers.clear();
            currentTaskId = getCurrentTaskId();
            renderAllMergedPaths();
        });
    }

    /**
     * 切换类卡片展开/折叠（带懒加载）
     * @param {string} cardId - 卡片 ID
     * @param {string} className - 类名
     */
    function toggleClassCard(cardId, className) {
        const content = document.getElementById(cardId);
        if (!content) return;
        
        const card = content.closest('.merged-class-card');
        const indicator = card?.querySelector('.merged-class-header > .expand-indicator');
        
        const isHidden = content.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(cardId);
            content.style.display = 'block';
            if (indicator) indicator.textContent = '▼';
            
            // 懒加载：展开时才请求 retainer 数据
            const retainersContainer = document.getElementById(`${cardId}-retainers`);
            if (retainersContainer && retainersContainer.innerHTML.trim() === '') {
                loadAndRenderRetainers(cardId, className);
            }
        } else {
            expandedNodes.delete(cardId);
            content.style.display = 'none';
            if (indicator) indicator.textContent = '▶';
        }
    }

    /**
     * 切换 retainer 节点展开/折叠（递归查 retainer 的 retainer）
     * @param {string} nodeId - 节点 ID
     * @param {string} className - 类名
     * @param {number} level - 当前层级
     */
    async function toggleRetainerNode(nodeId, className, level) {
        const childrenContainer = document.getElementById(`${nodeId}-children`);
        const nodeElement = document.querySelector(`[data-node-id="${nodeId}"]`);
        const indicator = nodeElement?.querySelector('.expand-indicator');
        
        if (!childrenContainer) return;
        
        // 深度限制
        if (level >= 5) {
            HeapCore.showNotification('已达到最大展开深度 (5层)', 'info');
            return;
        }

        // GC Root 类不再继续展开
        if (isGCRootClass(className)) {
            HeapCore.showNotification(`${Utils.getShortClassName(className)} 是 GC Root，无需继续展开`, 'info');
            return;
        }
        
        const isHidden = childrenContainer.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(nodeId);
            
            // 如果子节点还没有内容，按需加载
            if (childrenContainer.innerHTML.trim() === '') {
                childrenContainer.innerHTML = '<div class="loading-retainers" style="padding-left: 20px;">⏳ Loading...</div>';
                childrenContainer.style.display = 'block';
                if (indicator) indicator.textContent = '▼';

                const retainers = await fetchRetainersForClass(className);
                
                if (retainers.length === 0) {
                    childrenContainer.innerHTML = `<div class="no-retainers" style="padding-left: 20px;">
                        没有更多持有者数据
                    </div>`;
                    return;
                }

                const sortedRetainers = [...retainers].sort((a, b) => 
                    (b.retained_size || 0) - (a.retained_size || 0)
                );
                
                let childHtml = '';
                sortedRetainers.slice(0, 10).forEach((nested, nestedIndex) => {
                    childHtml += renderRetainerNode(nested, nodeId, nestedIndex, level + 1);
                });
                
                if (sortedRetainers.length > 10) {
                    childHtml += `<div class="more-retainers-hint" style="padding-left: ${(level + 1) * 20}px;">
                        还有 ${sortedRetainers.length - 10} 个持有者...
                    </div>`;
                }
                
                childrenContainer.innerHTML = childHtml;
            } else {
                childrenContainer.style.display = 'block';
                if (indicator) indicator.textContent = '▼';
            }
        } else {
            expandedNodes.delete(nodeId);
            childrenContainer.style.display = 'none';
            if (indicator) indicator.textContent = '▶';
        }
    }

    /**
     * 展开所有类卡片（不自动加载 retainers，避免并发请求风暴）
     */
    function expandAll() {
        document.querySelectorAll('.merged-class-card').forEach((card, index) => {
            const cardId = `merged-class-${index}`;
            const content = document.getElementById(cardId);
            const indicator = card.querySelector('.merged-class-header > .expand-indicator');
            const className = card.dataset.className;
            
            if (content && content.style.display === 'none') {
                expandedNodes.add(cardId);
                content.style.display = 'block';
                if (indicator) indicator.textContent = '▼';
                
                // 按需加载
                const retainersContainer = document.getElementById(`${cardId}-retainers`);
                if (retainersContainer && retainersContainer.innerHTML.trim() === '' && className) {
                    loadAndRenderRetainers(cardId, className);
                }
            }
        });
    }

    /**
     * 折叠所有节点
     */
    function collapseAll() {
        expandedNodes.clear();
        
        document.querySelectorAll('.merged-class-content').forEach(el => {
            el.style.display = 'none';
        });
        document.querySelectorAll('.retainer-children').forEach(el => {
            el.style.display = 'none';
        });
        document.querySelectorAll('.expand-indicator').forEach(el => {
            el.textContent = '▶';
        });
    }

    /**
     * 刷新视图
     */
    function refresh() {
        expandedNodes.clear();
        loadedRetainers.clear();
        currentTaskId = getCurrentTaskId();
        renderAllMergedPaths();
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        toggleClassCard,
        toggleRetainerNode,
        expandAll,
        collapseAll,
        refresh
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('mergedPaths', module);
    }

    return module;
})();

// 导出到全局
window.HeapMergedPaths = HeapMergedPaths;
