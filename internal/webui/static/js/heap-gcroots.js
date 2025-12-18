/**
 * Heap GC Roots Module
 * GC Roots 分析模块：负责 GC Root 的展示和分析
 * 
 * 职责：
 * - 从 gc_roots.json API 加载数据
 * - 渲染按类分组的 GC Roots 表格（类似 IDEA）
 * - 处理过滤和展开/折叠
 * - 支持展开查看具体实例和引用链
 */

const HeapGCRoots = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let gcRootsData = null;  // { summary: {...}, classes: [...] }
    let expandedClasses = new Set();  // 展开的类
    let expandedInstances = new Set();  // 展开的实例
    let isLoading = false;
    let currentTaskId = null;

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 从 API 加载 GC Roots 数据
     */
    async function loadGCRootsData(taskId) {
        if (isLoading) return;
        
        isLoading = true;
        currentTaskId = taskId;
        
        try {
            showLoadingState();
            const data = await API.getGCRootsSummary(taskId);
            gcRootsData = data;
            updateSummary(data.summary);
            renderTable(data.classes || []);
        } catch (error) {
            console.error('[HeapGCRoots] Failed to load GC roots:', error);
            showErrorState(error.message);
            // 回退到旧方式
            fallbackToLegacyData();
        } finally {
            isLoading = false;
        }
    }

    /**
     * 回退到旧的数据源
     */
    function fallbackToLegacyData() {
        const gcRootPaths = HeapCore.getState('gcRootPaths');
        const referenceGraphs = HeapCore.getState('referenceGraphs');
        const classData = HeapCore.getState('classData') || [];
        
        if (Object.keys(gcRootPaths).length === 0 && Object.keys(referenceGraphs).length === 0) {
            showEmptyState();
            return;
        }
        
        // 使用旧的构建逻辑
        const legacyData = buildLegacyGCRootsData(gcRootPaths, referenceGraphs, classData);
        gcRootsData = {
            summary: {
                total_roots: legacyData.length,
                total_classes: legacyData.length,
                total_retained: legacyData.reduce((sum, r) => sum + (r.total_retained || 0), 0),
                total_shallow: legacyData.reduce((sum, r) => sum + (r.total_shallow || 0), 0)
            },
            classes: legacyData
        };
        updateSummary(gcRootsData.summary);
        renderTable(legacyData);
    }

    /**
     * 旧的 GC Roots 构建逻辑
     */
    function buildLegacyGCRootsData(gcRootPaths, referenceGraphs, classData) {
        const classDataMap = new Map();
        classData.forEach(cls => {
            const name = cls.class_name || cls.name || '';
            if (name) classDataMap.set(name, cls);
        });
        
        const rootMap = new Map();

        for (const [className, paths] of Object.entries(gcRootPaths)) {
            for (const path of paths) {
                if (path.path && path.path.length > 0) {
                    const rootNode = path.path[0];
                    const rootType = path.root_type || 'Unknown';
                    const rootKey = `${rootType}:${rootNode.class_name}`;
                    
                    if (!rootMap.has(rootKey)) {
                        rootMap.set(rootKey, {
                            class_name: rootNode.class_name,
                            root_type: rootType,
                            total_shallow: rootNode.size || 0,
                            total_retained: 0,
                            instance_count: 1,
                            roots: []
                        });
                    }
                    
                    const root = rootMap.get(rootKey);
                    const classInfo = classData.find(c => (c.class_name || c.name) === className);
                    if (classInfo) {
                        root.total_retained += classInfo.retained_size || classInfo.total_size || 0;
                    }
                }
            }
        }

        return Array.from(rootMap.values())
            .sort((a, b) => b.total_retained - a.total_retained);
    }

    /**
     * 显示加载状态
     */
    function showLoadingState() {
        const tbody = document.getElementById('gcRootsTableBody');
        if (tbody) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" class="loading-state" style="text-align: center; padding: 40px;">
                        <div class="loading-spinner"></div>
                        <div style="margin-top: 10px;">Loading GC Roots data...</div>
                    </td>
                </tr>
            `;
        }
    }

    /**
     * 显示错误状态
     */
    function showErrorState(message) {
        const tbody = document.getElementById('gcRootsTableBody');
        if (tbody) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" class="error-state" style="text-align: center; padding: 40px; color: #f44336;">
                        <div class="icon">⚠️</div>
                        <div>Failed to load GC Roots: ${Utils.escapeHtml(message)}</div>
                        <div style="font-size: 12px; color: #808080; margin-top: 8px;">
                            Falling back to legacy data source...
                        </div>
                    </td>
                </tr>
            `;
        }
    }

    /**
     * 显示空状态
     */
    function showEmptyState() {
        const tbody = document.getElementById('gcRootsTableBody');
        if (tbody) {
            tbody.innerHTML = `
                <tr>
                    <td colspan="5" class="no-data-message" style="text-align: center; padding: 40px;">
                        <div class="icon">🌳</div>
                        <div>No GC Roots data available</div>
                        <div style="font-size: 12px; color: #808080; margin-top: 8px;">
                            GC Root analysis requires heap dump with dominator tree data
                        </div>
                    </td>
                </tr>
            `;
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
     * 获取 Root Type 的显示样式
     */
    function getRootTypeStyle(rootType) {
        const styles = {
            'STICKY_CLASS': { color: '#4caf50', icon: '📌' },
            'JAVA_FRAME': { color: '#2196f3', icon: '📚' },
            'THREAD_OBJECT': { color: '#ff9800', icon: '🧵' },
            'JNI_GLOBAL': { color: '#9c27b0', icon: '🔗' },
            'JNI_LOCAL': { color: '#673ab7', icon: '📍' },
            'MONITOR_USED': { color: '#f44336', icon: '🔒' },
            'NATIVE_STACK': { color: '#795548', icon: '📦' },
            'SYSTEM_CLASS': { color: '#607d8b', icon: '⚙️' },
            'UNKNOWN': { color: '#9e9e9e', icon: '❓' }
        };
        return styles[rootType] || styles['UNKNOWN'];
    }

    /**
     * 渲染 GC Roots 表格（按类分组）
     */
    function renderTable(classes) {
        const tbody = document.getElementById('gcRootsTableBody');
        if (!tbody) return;

        if (!classes || classes.length === 0) {
            showEmptyState();
            return;
        }

        const maxRetained = Math.max(...classes.map(c => c.total_retained || 0), 1);

        tbody.innerHTML = classes.map((cls, i) => {
            const retainedBarWidth = maxRetained > 0 ? ((cls.total_retained || 0) / maxRetained) * 100 : 0;
            const isExpanded = expandedClasses.has(cls.class_name);
            const isBusinessClass = checkIsBusinessClass(cls.class_name);
            const rootTypeStyle = getRootTypeStyle(cls.root_type);
            
            return `
                <tr class="gc-root-class-row ${isBusinessClass ? 'business-class' : ''}" 
                    onclick="HeapGCRoots.toggleClassRow('${Utils.escapeHtml(cls.class_name)}')">
                    <td>
                        <button class="expand-btn" id="gc-expand-${i}">
                            ${cls.roots && cls.roots.length > 0 ? (isExpanded ? '▼' : '▶') : '─'}
                        </button>
                    </td>
                    <td>
                        <span class="gc-root-type" style="color: ${rootTypeStyle.color};">
                            ${rootTypeStyle.icon} ${Utils.escapeHtml(cls.root_type || 'Unknown')}
                        </span>
                    </td>
                    <td>
                        <span class="gc-root-class ${isBusinessClass ? 'highlight' : ''}" 
                              title="${Utils.escapeHtml(cls.class_name)}">
                            ${isBusinessClass ? '🎯 ' : ''}${Utils.escapeHtml(Utils.getShortClassName(cls.class_name))}
                        </span>
                        <span class="instance-count">(${Utils.formatNumber(cls.instance_count || 0)} instances)</span>
                    </td>
                    <td>${Utils.formatBytes(cls.total_shallow || 0)}</td>
                    <td class="size-cell retained-cell">
                        <div class="size-bar-bg" style="width: ${retainedBarWidth}%"></div>
                        <span class="size-value">${Utils.formatBytes(cls.total_retained || 0)}</span>
                    </td>
                </tr>
                <tr id="gc-class-children-${i}" class="gc-root-instances" style="display: ${isExpanded ? 'table-row' : 'none'};">
                    <td colspan="5">
                        <div class="gc-root-instances-container">
                            ${isExpanded ? renderClassInstances(cls, i) : ''}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');
    }

    /**
     * 渲染类的实例列表
     */
    function renderClassInstances(cls, classIndex) {
        if (!cls.roots || cls.roots.length === 0) {
            return `<div class="no-instances">No instance data available</div>`;
        }

        const instances = cls.roots.slice(0, 50);  // 最多显示 50 个实例
        
        return `
            <div class="instances-header">
                <span>📋 ${cls.roots.length} GC Root instances</span>
                <span class="instances-hint">Click instance to view retained objects</span>
            </div>
            <table class="instances-table">
                <thead>
                    <tr>
                        <th style="width: 40px;"></th>
                        <th>Object ID</th>
                        <th>Shallow Size</th>
                        <th>Retained Size</th>
                        <th>Thread</th>
                    </tr>
                </thead>
                <tbody>
                    ${instances.map((inst, idx) => {
                        const instanceKey = `${cls.class_name}:${inst.object_id}`;
                        const isInstExpanded = expandedInstances.has(instanceKey);
                        
                        return `
                            <tr class="instance-row" onclick="HeapGCRoots.toggleInstanceRow('${Utils.escapeHtml(cls.class_name)}', '${Utils.escapeHtml(inst.object_id)}', ${classIndex}, ${idx}); event.stopPropagation();">
                                <td>
                                    <button class="expand-btn mini" id="gc-inst-expand-${classIndex}-${idx}">
                                        ${isInstExpanded ? '▼' : '▶'}
                                    </button>
                                </td>
                                <td>
                                    <code class="object-id">${Utils.escapeHtml(inst.object_id)}</code>
                                </td>
                                <td>${Utils.formatBytes(inst.shallow_size || 0)}</td>
                                <td class="retained-size">${Utils.formatBytes(inst.retained_size || 0)}</td>
                                <td>
                                    ${inst.thread_id && inst.thread_id !== '0x0' ? 
                                        `<code class="thread-id">${Utils.escapeHtml(inst.thread_id)}</code>` : 
                                        '<span class="no-thread">-</span>'}
                                </td>
                            </tr>
                            <tr id="gc-inst-children-${classIndex}-${idx}" class="instance-children" style="display: ${isInstExpanded ? 'table-row' : 'none'};">
                                <td colspan="5">
                                    <div id="gc-inst-content-${classIndex}-${idx}" class="instance-content">
                                        ${isInstExpanded ? '<div class="loading">Loading fields...</div>' : ''}
                                    </div>
                                </td>
                            </tr>
                        `;
                    }).join('')}
                </tbody>
            </table>
            ${cls.roots.length > 50 ? `<div class="more-instances">... and ${cls.roots.length - 50} more instances</div>` : ''}
        `;
    }

    /**
     * 加载实例的字段数据
     */
    async function loadInstanceFields(objectId, classIndex, instIndex) {
        const contentDiv = document.getElementById(`gc-inst-content-${classIndex}-${instIndex}`);
        if (!contentDiv) return;

        contentDiv.innerHTML = '<div class="loading">Loading fields...</div>';

        try {
            const fields = await API.getObjectFields(currentTaskId, objectId);
            renderInstanceFields(contentDiv, fields, objectId);
        } catch (error) {
            console.error('[HeapGCRoots] Failed to load fields:', error);
            contentDiv.innerHTML = `
                <div class="error-message">
                    Failed to load fields: ${Utils.escapeHtml(error.message)}
                </div>
            `;
        }
    }

    /**
     * 渲染实例字段
     */
    function renderInstanceFields(container, fields, parentId) {
        if (!fields || fields.length === 0) {
            container.innerHTML = '<div class="no-fields">No fields available</div>';
            return;
        }

        container.innerHTML = `
            <div class="fields-list">
                ${fields.map((field, idx) => {
                    const hasChildren = field.has_children && field.ref_id;
                    const isBusinessClass = field.ref_class ? checkIsBusinessClass(field.ref_class) : false;
                    
                    return `
                        <div class="field-item ${hasChildren ? 'expandable' : ''} ${isBusinessClass ? 'business-class' : ''}">
                            <span class="field-expand">${hasChildren ? '▶' : '─'}</span>
                            <span class="field-name">${Utils.escapeHtml(field.name)}</span>
                            <span class="field-type">${Utils.escapeHtml(field.type)}</span>
                            ${field.ref_class ? `
                                <span class="field-ref-class ${isBusinessClass ? 'highlight' : ''}" 
                                      title="${Utils.escapeHtml(field.ref_class)}">
                                    → ${Utils.escapeHtml(Utils.getShortClassName(field.ref_class))}
                                </span>
                            ` : ''}
                            ${field.value !== undefined && field.value !== null ? `
                                <span class="field-value">${Utils.escapeHtml(String(field.value))}</span>
                            ` : ''}
                            ${field.retained_size ? `
                                <span class="field-size">${Utils.formatBytes(field.retained_size)}</span>
                            ` : ''}
                        </div>
                    `;
                }).join('')}
            </div>
        `;
    }

    /**
     * 更新汇总信息
     */
    function updateSummary(summary) {
        const summaryCount = document.getElementById('gcRootsTotalCount');
        const summarySize = document.getElementById('gcRootsRetainedSize');
        const summaryClasses = document.getElementById('gcRootsClassCount');
        
        if (summary) {
            if (summaryCount) summaryCount.textContent = Utils.formatNumber(summary.total_roots || 0);
            if (summarySize) summarySize.textContent = Utils.formatBytes(summary.total_retained || 0);
            if (summaryClasses) summaryClasses.textContent = Utils.formatNumber(summary.total_classes || 0);
        }
    }

    // ============================================
    // 公共方法
    // ============================================
    
    /**
     * 初始化模块
     */
    function init() {
        // 监听数据加载事件
        HeapCore.on('dataLoaded', function(data) {
            expandedClasses.clear();
            expandedInstances.clear();
            
            // 获取当前 taskId
            const taskId = getCurrentTaskId();
            if (taskId) {
                loadGCRootsData(taskId);
            } else {
                fallbackToLegacyData();
            }
        });

        // 监听 retainer 数据更新
        HeapCore.on('retainerDataUpdated', function() {
            if (!gcRootsData) {
                fallbackToLegacyData();
            }
        });
    }

    /**
     * 获取当前 taskId（多种来源）
     */
    function getCurrentTaskId() {
        // 1. 尝试从 App 模块获取
        if (typeof App !== 'undefined' && App.getCurrentTask) {
            const taskId = App.getCurrentTask();
            if (taskId) return taskId;
        }
        // 2. 尝试从 URL 获取
        const urlParams = new URLSearchParams(window.location.search);
        const urlTaskId = urlParams.get('task');
        if (urlTaskId) return urlTaskId;
        // 3. 尝试从全局变量获取
        if (window.currentTaskId) return window.currentTaskId;
        return null;
    }

    /**
     * 渲染 GC Roots（外部调用入口）
     */
    function render() {
        const taskId = getCurrentTaskId();
        if (taskId) {
            loadGCRootsData(taskId);
        } else {
            fallbackToLegacyData();
        }
    }

    /**
     * 过滤 GC Roots
     */
    function filter() {
        if (!gcRootsData || !gcRootsData.classes) return;
        
        const searchTerm = document.getElementById('gcRootsSearch')?.value?.toLowerCase() || '';
        const typeFilter = document.getElementById('gcRootsTypeFilter')?.value || '';
        
        let filtered = gcRootsData.classes;
        
        if (searchTerm) {
            filtered = filtered.filter(cls => 
                cls.class_name.toLowerCase().includes(searchTerm) ||
                (cls.root_type && cls.root_type.toLowerCase().includes(searchTerm))
            );
        }
        
        if (typeFilter) {
            filtered = filtered.filter(cls => cls.root_type === typeFilter);
        }
        
        renderTable(filtered);
    }

    /**
     * 切换类行展开/折叠
     */
    function toggleClassRow(className) {
        const classes = gcRootsData?.classes || [];
        const classIndex = classes.findIndex(c => c.class_name === className);
        if (classIndex === -1) return;
        
        const cls = classes[classIndex];
        const childrenRow = document.getElementById(`gc-class-children-${classIndex}`);
        const expandBtn = document.getElementById(`gc-expand-${classIndex}`);
        
        if (!childrenRow) return;
        
        const isVisible = childrenRow.style.display !== 'none';
        
        if (isVisible) {
            expandedClasses.delete(className);
            childrenRow.style.display = 'none';
            if (expandBtn) expandBtn.textContent = '▶';
        } else {
            expandedClasses.add(className);
            childrenRow.style.display = 'table-row';
            if (expandBtn) expandBtn.textContent = '▼';
            
            // 渲染实例列表
            const container = childrenRow.querySelector('.gc-root-instances-container');
            if (container && container.innerHTML.trim() === '') {
                container.innerHTML = renderClassInstances(cls, classIndex);
            }
        }
    }

    /**
     * 切换实例行展开/折叠
     */
    function toggleInstanceRow(className, objectId, classIndex, instIndex) {
        const instanceKey = `${className}:${objectId}`;
        const childrenRow = document.getElementById(`gc-inst-children-${classIndex}-${instIndex}`);
        const expandBtn = document.getElementById(`gc-inst-expand-${classIndex}-${instIndex}`);
        
        if (!childrenRow) return;
        
        const isVisible = childrenRow.style.display !== 'none';
        
        if (isVisible) {
            expandedInstances.delete(instanceKey);
            childrenRow.style.display = 'none';
            if (expandBtn) expandBtn.textContent = '▶';
        } else {
            expandedInstances.add(instanceKey);
            childrenRow.style.display = 'table-row';
            if (expandBtn) expandBtn.textContent = '▼';
            
            // 加载字段数据
            loadInstanceFields(objectId, classIndex, instIndex);
        }
    }

    /**
     * 获取 GC Roots 数据
     */
    function getData() {
        return gcRootsData;
    }

    /**
     * 刷新数据
     */
    function refresh() {
        const taskId = getCurrentTaskId();
        if (taskId) {
            gcRootsData = null;
            expandedClasses.clear();
            expandedInstances.clear();
            loadGCRootsData(taskId);
        }
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        filter,
        toggleClassRow,
        toggleInstanceRow,
        getData,
        refresh
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('gcroots', module);
    }

    return module;
})();

// 导出到全局
window.HeapGCRoots = HeapGCRoots;
