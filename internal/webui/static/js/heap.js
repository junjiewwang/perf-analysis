/**
 * Heap Analysis Module (Facade)
 * 堆分析门面模块：统一对外接口，协调各子模块
 * 
 * 架构说明：
 * - HeapCore: 核心模块，状态管理和事件系统
 * - HeapDiagnosis: 问题诊断概览（首页展示）
 * - HeapTreemap: Treemap 可视化
 * - HeapHistogram: Class Histogram 表格
 * - HeapGCRoots: GC Roots 分析
 * - HeapMergedPaths: Merged Paths 分析（IDEA 风格）
 * - HeapRootCause: Root Cause 详细分析
 * 
 * 设计原则：
 * - 门面模式：提供统一的简化接口
 * - 高内聚低耦合：各模块独立，通过事件通信
 * - 开放封闭：易于扩展新模块，无需修改现有代码
 */

const HeapAnalysis = (function() {
    'use strict';

    // ============================================
    // 初始化
    // ============================================
    
    /**
     * 初始化所有模块
     */
    function init() {
        // 初始化核心模块
        HeapCore.init();
        
        // 子模块会在加载时自动注册到核心模块
        console.log('[HeapAnalysis] Initialized with modules:', 
            Array.from(['diagnosis', 'treemap', 'histogram', 'gcroots', 'mergedPaths', 'rootcause'])
                .filter(name => HeapCore.getModule(name))
                .join(', ')
        );
    }

    // ============================================
    // Overview 渲染
    // ============================================
    
    /**
     * 渲染概览面板
     * @param {Object} data - 摘要数据
     */
    function renderOverview(data) {
        const heapData = data.data || {};

        document.getElementById('totalSamples').textContent = Utils.formatBytes(heapData.total_heap_size || 0);
        document.getElementById('topFuncsCount').textContent = heapData.total_classes || 0;
        document.getElementById('threadsCount').textContent = Utils.formatNumber(heapData.total_instances || 0);
        document.getElementById('taskUUID').textContent = data.task_uuid || '-';

        const statLabels = document.querySelectorAll('.stat-label');
        if (statLabels.length >= 3) {
            statLabels[0].textContent = 'Total Heap Size';
            statLabels[1].textContent = 'Total Classes';
            statLabels[2].textContent = 'Total Instances';
        }

        // 渲染 top classes 预览
        const topItems = data.top_items || [];
        const previewBody = document.getElementById('topFuncsPreview');
        previewBody.innerHTML = topItems.slice(0, 5).map((item, i) => `
            <tr>
                <td>${i + 1}</td>
                <td class="func-name" title="${Utils.escapeHtml(item.name)}">${Utils.escapeHtml(item.name)}</td>
                <td>
                    <div class="percentage-bar">
                        <div class="percentage-bar-fill" style="width: ${Math.min(item.percentage, 100)}%; background: linear-gradient(90deg, #9b59b6 0%, #8e44ad 100%);"></div>
                    </div>
                    ${item.percentage.toFixed(2)}%
                </td>
                <td>
                    <span style="font-size: 12px; color: #666;">${Utils.formatBytes(item.value)}</span>
                </td>
            </tr>
        `).join('');

        const cardTitle = document.querySelector('#overview .card h2');
        if (cardTitle) cardTitle.textContent = 'Top 5 Classes by Memory';

        const tips = document.querySelector('#overview .card .tips');
        if (tips) tips.innerHTML = '<span>💡 Click on Memory Map or Class Histogram tabs for detailed analysis</span>';

        document.getElementById('threadList').innerHTML = '<li class="thread-item"><span class="thread-name">N/A for Heap Analysis</span></li>';
    }

    // ============================================
    // 分析渲染
    // ============================================
    
    /**
     * 渲染分析数据
     * @param {Object} data - 分析数据
     */
    function renderAnalysis(data) {
        const heapData = data.data || {};

        // 渲染统计信息
        document.getElementById('heapTotalSize').textContent = heapData.heap_size_human || Utils.formatBytes(heapData.total_heap_size || 0);
        document.getElementById('heapTotalClasses').textContent = Utils.formatNumber(heapData.total_classes || 0);
        document.getElementById('heapTotalInstances').textContent = Utils.formatNumber(heapData.total_instances || 0);
        document.getElementById('heapFormat').textContent = heapData.format || 'Unknown';

        // 加载数据到核心模块（触发各子模块渲染）
        HeapCore.loadAnalysisData(data);

        // 渲染问题诊断概览（首页）
        const diagnosisModule = HeapCore.getModule('diagnosis');
        if (diagnosisModule) {
            diagnosisModule.render(data);
        }

        // Treemap 需要额外的参数
        const treemapModule = HeapCore.getModule('treemap');
        if (treemapModule) {
            treemapModule.render(data.top_items || [], heapData.total_heap_size || 0);
        }

        // 渲染 GC Roots
        const gcRootsModule = HeapCore.getModule('gcroots');
        if (gcRootsModule) {
            gcRootsModule.render();
        }
    }

    // ============================================
    // 委托方法 - Diagnosis
    // ============================================
    
    function renderDiagnosis(data) {
        const diagnosisModule = HeapCore.getModule('diagnosis');
        if (diagnosisModule) {
            diagnosisModule.render(data);
        }
    }

    function getDiagnosisData() {
        const diagnosisModule = HeapCore.getModule('diagnosis');
        return diagnosisModule ? diagnosisModule.getDiagnosisData() : null;
    }

    // ============================================
    // 委托方法 - Histogram
    // ============================================
    
    function filterClasses() {
        const searchTerm = document.getElementById('heapClassSearch')?.value || '';
        HeapHistogram.filter(searchTerm);
    }

    function clearSearch() {
        HeapHistogram.clearSearch();
    }

    function setViewMode(mode) {
        HeapHistogram.setViewMode(mode);
    }

    function toggleHistogramRow(idx) {
        HeapHistogram.toggleRow(idx);
    }

    function sortHistogram(field) {
        HeapHistogram.sort(field);
    }

    function togglePackage(idx) {
        HeapHistogram.togglePackage(idx);
    }

    function searchClass(className) {
        HeapHistogram.searchClass(className);
    }

    // ============================================
    // 委托方法 - GC Roots
    // ============================================
    
    function toggleGCRootRow(idx) {
        HeapGCRoots.toggleRow(idx);
    }

    function filterGCRoots() {
        HeapGCRoots.filter();
    }

    // ============================================
    // 委托方法 - Merged Paths
    // ============================================
    
    function expandAllPaths() {
        HeapMergedPaths.expandAll();
    }

    function collapseAllPaths() {
        HeapMergedPaths.collapseAll();
    }

    // ============================================
    // 委托方法 - Root Cause
    // ============================================
    
    function renderRootCauseAnalysis(data) {
        HeapRootCause.render(data);
    }

    function toggleBusinessGroup(idx) {
        HeapRootCause.toggleBusinessGroup(idx);
    }

    function filterRootCause() {
        HeapRootCause.filter();
    }

    // ============================================
    // 委托方法 - Treemap
    // ============================================
    
    function resizeTreemap() {
        const treemapModule = HeapCore.getModule('treemap');
        if (treemapModule) {
            treemapModule.resize();
        }
    }

    // ============================================
    // 兼容性方法（保持向后兼容）
    // ============================================
    
    function renderTreemap(topItems, totalSize) {
        const treemapModule = HeapCore.getModule('treemap');
        if (treemapModule) {
            treemapModule.render(topItems, totalSize);
        }
    }

    function renderHistogram(data) {
        HeapHistogram.render(data);
    }

    function renderGCRoots() {
        HeapGCRoots.render();
    }

    function getClassData() {
        return HeapCore.getState('classData');
    }

    // 旧版兼容方法
    function toggleRetainers(idx) {
        toggleHistogramRow(idx);
    }

    function toggleBusinessRetainers(idx) {
        toggleHistogramRow(idx);
    }

    function showSearchNotification(message, type) {
        HeapCore.showNotification(message, type);
    }

    function formatClassNameIDEA(fullName) {
        return HeapCore.formatClassNameIDEA(fullName);
    }

    // ============================================
    // 公共 API
    // ============================================
    
    return {
        // 初始化
        init,
        
        // Overview
        renderOverview,
        
        // 分析
        renderAnalysis,
        
        // Diagnosis (新增)
        renderDiagnosis,
        getDiagnosisData,
        
        // Histogram
        filterClasses,
        clearSearch,
        setViewMode,
        toggleHistogramRow,
        sortHistogram,
        togglePackage,
        searchClass,
        renderHistogram,
        
        // GC Roots
        toggleGCRootRow,
        filterGCRoots,
        renderGCRoots,
        
        // Merged Paths
        expandAllPaths,
        collapseAllPaths,
        
        // Root Cause
        renderRootCauseAnalysis,
        toggleBusinessGroup,
        filterRootCause,
        
        // Treemap
        resizeTreemap,
        renderTreemap,
        
        // 工具方法
        getClassData,
        formatClassNameIDEA,
        showSearchNotification,
        
        // 兼容性方法
        toggleRetainers,
        toggleBusinessRetainers
    };
})();

// 导出到全局
window.HeapAnalysis = HeapAnalysis;
