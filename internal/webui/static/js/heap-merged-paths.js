/**
 * Heap Merged Paths Module
 * Merged Paths 分析模块：IDEA 风格的合并路径展示
 * 
 * 职责：
 * - 直接展示所有 Top 类的持有者路径（无需选择）
 * - 使用 retainers 数据构建持有者树
 * - 渲染类似 IDEA Memory Profiler 的树视图
 * - 处理展开/折叠操作
 * - 支持递归展开 retainer（查看 retainer 的 retainer）
 * 
 * 数据结构说明：
 * - classData: 包含 retainers 的类数据
 * - retainers: 数组，每个元素包含 retainer_class, field_name, retained_size 等
 */

const HeapMergedPaths = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let expandedNodes = new Set();
    let loadedRetainers = new Map(); // 缓存已加载的 retainer 数据
    let classDataMap = new Map(); // 类名 -> 类数据的映射

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 获取有 retainers 的类列表（按内存大小排序）
     * @returns {Array} 有 retainers 的类列表
     */
    function getClassesWithRetainers() {
        const classData = HeapCore.getState('classData') || [];
        
        // 构建类名映射
        classDataMap.clear();
        classData.forEach(cls => {
            const name = cls.class_name || cls.name || '';
            if (name) {
                classDataMap.set(name, cls);
            }
        });
        
        // 筛选有 retainers 的类，按内存大小排序
        return classData
            .map(cls => {
                const retainers = cls.retainers || [];
                return {
                    ...cls,
                    retainers: retainers,
                    retainerCount: retainers.length
                };
            })
            .filter(cls => cls.retainerCount > 0)
            .sort((a, b) => b.size - a.size);
    }

    /**
     * 查找某个类的 retainers
     * @param {string} className - 类名
     * @returns {Array} retainers 数组
     */
    function findRetainersForClass(className) {
        // 先从缓存查找
        if (loadedRetainers.has(className)) {
            return loadedRetainers.get(className);
        }
        
        // 从类数据中查找
        const classInfo = classDataMap.get(className);
        if (classInfo && classInfo.retainers) {
            loadedRetainers.set(className, classInfo.retainers);
            return classInfo.retainers;
        }
        
        // 尝试模糊匹配（短类名）
        const shortName = Utils.getShortClassName(className);
        for (const [name, cls] of classDataMap) {
            if (Utils.getShortClassName(name) === shortName && cls.retainers) {
                loadedRetainers.set(className, cls.retainers);
                return cls.retainers;
            }
        }
        
        return [];
    }

    /**
     * 渲染单个 retainer 节点（支持递归展开）
     * @param {Object} retainer - retainer 对象
     * @param {string} parentId - 父节点 ID
     * @param {number} index - 索引
     * @param {number} level - 嵌套层级
     * @returns {string} HTML 字符串
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
        
        // 检查这个 retainer 是否有自己的 retainers
        const hasNestedRetainers = findRetainersForClass(retainerClass).length > 0;
        const isGCRoot = isGCRootClass(retainerClass);
        
        // 计算缩进
        const indent = level * 20;
        
        let html = `
            <div class="retainer-node level-${level}" data-node-id="${nodeId}" data-class="${Utils.escapeHtml(retainerClass)}" style="padding-left: ${indent}px;">
                <div class="retainer-row ${hasNestedRetainers ? 'expandable' : ''} ${isGCRoot ? 'gc-root' : ''}" 
                     onclick="HeapMergedPaths.toggleRetainerNode('${nodeId}', '${Utils.escapeHtml(retainerClass).replace(/'/g, "\\'")}', ${level})">
                    <span class="expand-indicator">${hasNestedRetainers ? (isExpanded ? '▼' : '▶') : '─'}</span>
                    <span class="retainer-icon">${isGCRoot ? '🌳' : '📦'}</span>
                    <span class="retainer-class" title="${Utils.escapeHtml(retainerClass)}">${Utils.escapeHtml(shortName)}</span>
                    ${fieldName ? `<span class="retainer-field">.${Utils.escapeHtml(fieldName)}</span>` : ''}
                    <span class="retainer-stats">
                        <span class="stat-percentage" title="占比">${percentage.toFixed(1)}%</span>
                        <span class="stat-size" title="保留大小">${Utils.formatBytes(retainedSize)}</span>
                        <span class="stat-count" title="保留对象数">×${retainedCount.toLocaleString()}</span>
                    </span>
                    ${isGCRoot ? '<span class="gc-root-badge">GC Root</span>' : ''}
                </div>
                <div id="${nodeId}-children" class="retainer-children" style="display: ${isExpanded ? 'block' : 'none'};">
        `;
        
        // 如果已展开，渲染子节点
        if (isExpanded && hasNestedRetainers) {
            const nestedRetainers = findRetainersForClass(retainerClass);
            const sortedNested = [...nestedRetainers].sort((a, b) => 
                (b.retained_size || 0) - (a.retained_size || 0)
            );
            
            // 限制深度，避免无限递归
            if (level < 5) {
                sortedNested.slice(0, 10).forEach((nested, nestedIndex) => {
                    html += renderRetainerNode(nested, nodeId, nestedIndex, level + 1);
                });
                
                if (sortedNested.length > 10) {
                    html += `<div class="more-retainers-hint" style="padding-left: ${(level + 1) * 20}px;">
                        还有 ${sortedNested.length - 10} 个持有者...
                    </div>`;
                }
            } else {
                html += `<div class="max-depth-hint" style="padding-left: ${(level + 1) * 20}px;">
                    ⚠️ 已达到最大展开深度
                </div>`;
            }
        }
        
        html += '</div></div>';
        return html;
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
     * 渲染 retainers 树
     * @param {Array} retainers - retainers 数组
     * @param {string} cardId - 卡片 ID
     * @returns {string} HTML 字符串
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
     * @param {Object} classInfo - 类信息
     * @param {number} index - 索引
     * @returns {string} HTML 字符串
     */
    function renderClassCard(classInfo, index) {
        const retainers = classInfo.retainers || [];
        if (retainers.length === 0) return '';
        
        const cardId = `merged-class-${index}`;
        const isExpanded = expandedNodes.has(cardId);
        const className = classInfo.class_name || classInfo.name || '';
        const shortName = Utils.getShortClassName(className);
        
        return `
            <div class="merged-class-card" data-class-name="${Utils.escapeHtml(className)}">
                <div class="merged-class-header" onclick="HeapMergedPaths.toggleClassCard('${cardId}')">
                    <span class="expand-indicator">${isExpanded ? '▼' : '▶'}</span>
                    <span class="class-icon">🎯</span>
                    <span class="class-name" title="${Utils.escapeHtml(className)}">${Utils.escapeHtml(shortName)}</span>
                    <span class="class-stats">
                        <span class="stat-item" title="实例数量">
                            📊 ${(classInfo.instance_count || classInfo.instanceCount || classInfo.count || 0).toLocaleString()} instances
                        </span>
                        <span class="stat-item" title="浅层大小">
                            💾 ${Utils.formatBytes(classInfo.total_size || classInfo.size || 0)}
                        </span>
                        <span class="stat-item" title="Retainer 数量">
                            🔗 ${retainers.length} retainers
                        </span>
                    </span>
                </div>
                <div id="${cardId}" class="merged-class-content" style="display: ${isExpanded ? 'block' : 'none'};">
                    <div class="retainers-header">
                        <span class="header-title">📍 Retained by (谁持有这个类的实例)</span>
                        <span class="header-hint">💡 点击类名展开查看详细的持有者列表</span>
                    </div>
                    ${renderRetainersTree(retainers, cardId)}
                </div>
            </div>
        `;
    }

    /**
     * 渲染所有类的 Merged Paths
     */
    function renderAllMergedPaths() {
        const container = document.getElementById('mergedPathsContainer');
        if (!container) return;

        const classesWithRetainers = getClassesWithRetainers();
        
        console.log('[HeapMergedPaths] Classes with retainers:', classesWithRetainers.length);
        
        if (classesWithRetainers.length === 0) {
            container.innerHTML = `
                <div class="no-data-message">
                    <div class="icon">🔀</div>
                    <div class="title">没有找到 Retainer 数据</div>
                    <div class="hint">
                        Retainer 数据显示哪些类持有目标类的实例。<br>
                        请确保分析数据中包含 retainers 信息。
                    </div>
                </div>
            `;
            return;
        }

        // 计算统计信息
        const totalRetainers = classesWithRetainers.reduce((sum, cls) => sum + cls.retainerCount, 0);

        let html = `
            <div class="merged-paths-summary">
                <div class="summary-stat">
                    <span class="stat-value">${classesWithRetainers.length}</span>
                    <span class="stat-label">Classes with Retainers</span>
                </div>
                <div class="summary-stat">
                    <span class="stat-value">${totalRetainers}</span>
                    <span class="stat-label">Total Retainer Paths</span>
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
                <span>🔍 点击类名展开查看详细的持有者列表</span>
                <span>📊 按保留内存大小排序</span>
            </div>
            <div class="merged-classes-list">
        `;

        // 渲染每个类的卡片（最多 30 个）
        classesWithRetainers.slice(0, 30).forEach((cls, index) => {
            html += renderClassCard(cls, index);
        });

        if (classesWithRetainers.length > 30) {
            html += `
                <div class="more-classes-hint">
                    还有 ${classesWithRetainers.length - 30} 个类未显示，请在 Class Histogram 中查看完整列表
                </div>
            `;
        }

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
            renderAllMergedPaths();
        });
    }

    /**
     * 切换类卡片展开/折叠
     * @param {string} cardId - 卡片 ID
     */
    function toggleClassCard(cardId) {
        const content = document.getElementById(cardId);
        if (!content) return;
        
        const card = content.closest('.merged-class-card');
        const indicator = card?.querySelector('.merged-class-header > .expand-indicator');
        
        const isHidden = content.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(cardId);
            content.style.display = 'block';
            if (indicator) indicator.textContent = '▼';
        } else {
            expandedNodes.delete(cardId);
            content.style.display = 'none';
            if (indicator) indicator.textContent = '▶';
        }
    }

    /**
     * 切换 retainer 节点展开/折叠（递归展开）
     * @param {string} nodeId - 节点 ID
     * @param {string} className - 类名
     * @param {number} level - 当前层级
     */
    function toggleRetainerNode(nodeId, className, level) {
        const childrenContainer = document.getElementById(`${nodeId}-children`);
        const nodeElement = document.querySelector(`[data-node-id="${nodeId}"]`);
        const indicator = nodeElement?.querySelector('.expand-indicator');
        
        if (!childrenContainer) return;
        
        // 检查是否有可展开的内容
        const retainers = findRetainersForClass(className);
        if (retainers.length === 0) {
            HeapCore.showNotification(`${Utils.getShortClassName(className)} 没有更多持有者数据`, 'info');
            return;
        }
        
        const isHidden = childrenContainer.style.display === 'none';
        
        if (isHidden) {
            expandedNodes.add(nodeId);
            
            // 如果子节点还没有内容，动态渲染
            if (childrenContainer.innerHTML.trim() === '') {
                const sortedRetainers = [...retainers].sort((a, b) => 
                    (b.retained_size || 0) - (a.retained_size || 0)
                );
                
                if (level < 5) {
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
                    childrenContainer.innerHTML = `<div class="max-depth-hint" style="padding-left: ${(level + 1) * 20}px;">
                        ⚠️ 已达到最大展开深度 (5层)
                    </div>`;
                }
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
     * 展开所有节点
     */
    function expandAll() {
        document.querySelectorAll('.merged-class-card').forEach((card, index) => {
            const cardId = `merged-class-${index}`;
            const content = document.getElementById(cardId);
            const indicator = card.querySelector('.merged-class-header > .expand-indicator');
            
            if (content) {
                expandedNodes.add(cardId);
                content.style.display = 'block';
                if (indicator) indicator.textContent = '▼';
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
