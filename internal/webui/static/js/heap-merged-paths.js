/**
 * Heap Merged Paths Module
 * Merged Paths 分析模块：IDEA 风格的合并路径展示
 * 
 * 职责：
 * - 直接展示所有 Top 类的持有者路径（无需选择）
 * - 使用 retainers 数据构建持有者树
 * - 渲染类似 IDEA Memory Profiler 的树视图
 * - 处理展开/折叠操作
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

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 获取有 retainers 的类列表（按内存大小排序）
     * @returns {Array} 有 retainers 的类列表
     */
    function getClassesWithRetainers() {
        const classData = HeapCore.getState('classData') || [];
        
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
     * 渲染单个类的 retainers 树
     * @param {Array} retainers - retainers 数组
     * @param {string} targetClassName - 目标类名
     * @param {string} cardId - 卡片 ID
     * @returns {string} HTML 字符串
     */
    function renderRetainersTree(retainers, targetClassName, cardId) {
        if (!retainers || retainers.length === 0) {
            return '<div class="no-retainers">没有 retainer 数据</div>';
        }

        // 按 retained_size 排序
        const sortedRetainers = [...retainers].sort((a, b) => 
            (b.retained_size || 0) - (a.retained_size || 0)
        );

        let html = '<div class="retainers-tree">';
        
        sortedRetainers.forEach((retainer, index) => {
            const retainerClass = retainer.retainer_class || retainer.class_name || 'Unknown';
            const fieldName = retainer.field_name || '';
            const retainedSize = retainer.retained_size || 0;
            const retainedCount = retainer.retained_count || 0;
            const percentage = retainer.percentage || 0;
            const depth = retainer.depth || 1;
            
            const shortName = Utils.getShortClassName(retainerClass);
            const nodeId = `${cardId}-retainer-${index}`;
            
            html += `
                <div class="retainer-node" data-node-id="${nodeId}">
                    <div class="retainer-row">
                        <span class="retainer-depth" title="引用深度">${'─'.repeat(Math.min(depth, 3))}▶</span>
                        <span class="retainer-icon">📦</span>
                        <span class="retainer-class" title="${Utils.escapeHtml(retainerClass)}">${Utils.escapeHtml(shortName)}</span>
                        ${fieldName ? `<span class="retainer-field">.${Utils.escapeHtml(fieldName)}</span>` : ''}
                        <span class="retainer-stats">
                            <span class="stat-percentage" title="占比">${percentage.toFixed(1)}%</span>
                            <span class="stat-size" title="保留大小">${Utils.formatBytes(retainedSize)}</span>
                            <span class="stat-count" title="保留对象数">×${retainedCount.toLocaleString()}</span>
                        </span>
                    </div>
                </div>
            `;
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
        const shortName = Utils.getShortClassName(classInfo.name);
        
        // 计算总 retained size
        const totalRetainedSize = retainers.reduce((sum, r) => sum + (r.retained_size || 0), 0);
        
        return `
            <div class="merged-class-card" data-class-name="${Utils.escapeHtml(classInfo.name)}">
                <div class="merged-class-header" onclick="HeapMergedPaths.toggleClassCard('${cardId}')">
                    <span class="expand-indicator">${isExpanded ? '▼' : '▶'}</span>
                    <span class="class-icon">🎯</span>
                    <span class="class-name" title="${Utils.escapeHtml(classInfo.name)}">${Utils.escapeHtml(shortName)}</span>
                    <span class="class-stats">
                        <span class="stat-item" title="实例数量">
                            📊 ${(classInfo.instanceCount || classInfo.count || 0).toLocaleString()} instances
                        </span>
                        <span class="stat-item" title="浅层大小">
                            💾 ${Utils.formatBytes(classInfo.size || 0)}
                        </span>
                        <span class="stat-item" title="Retainer 数量">
                            🔗 ${retainers.length} retainers
                        </span>
                    </span>
                </div>
                <div id="${cardId}" class="merged-class-content" style="display: ${isExpanded ? 'block' : 'none'};">
                    <div class="retainers-header">
                        <span class="header-title">📍 Retained by (谁持有这个类的实例)</span>
                        <span class="header-hint">按保留大小排序</span>
                    </div>
                    ${renderRetainersTree(retainers, classInfo.name, cardId)}
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
        const indicator = card?.querySelector('.expand-indicator');
        
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
     * 展开所有节点
     */
    function expandAll() {
        document.querySelectorAll('.merged-class-card').forEach((card, index) => {
            const cardId = `merged-class-${index}`;
            const content = document.getElementById(cardId);
            const indicator = card.querySelector('.expand-indicator');
            
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
        document.querySelectorAll('.expand-indicator').forEach(el => {
            el.textContent = '▶';
        });
    }

    /**
     * 刷新视图
     */
    function refresh() {
        expandedNodes.clear();
        renderAllMergedPaths();
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        toggleClassCard,
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
