/**
 * Heap Retained Treemap Module
 * Retained Size Treemap 可视化：面积 = retained size，颜色 = 类分组
 * 
 * 功能：
 * - ECharts Treemap 展示 retained size 分布
 * - 按类分组的层次结构
 * - 点击钻取（进入子 dominator 树）
 * - 面包屑导航返回上层
 */

const HeapRetainedTreemap = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================

    let chart = null;
    let containerElement = null;
    let currentTaskId = '';
    let currentRootId = '';
    let navigationStack = []; // For drill-down navigation

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

    // ============================================
    // API 调用
    // ============================================

    async function fetchTreemapData(rootId, maxNodes = 200) {
        try {
            let url = `/api/refgraph/treemap?maxNodes=${maxNodes}`;
            if (currentTaskId) {
                url += `&task=${encodeURIComponent(currentTaskId)}`;
            }
            if (rootId) {
                url += `&root=${encodeURIComponent(rootId)}`;
            }
            const response = await fetch(url);
            if (!response.ok) {
                console.warn(`[RetainedTreemap] Failed to fetch data: ${response.status}`);
                return [];
            }
            return await response.json();
        } catch (error) {
            console.error('[RetainedTreemap] Error fetching treemap data:', error);
            return [];
        }
    }

    // ============================================
    // 渲染
    // ============================================

    function generateChartOption(treeData) {
        return {
            tooltip: {
                formatter: function(info) {
                    const data = info.data;
                    if (!data) return '';
                    const name = data.name || '';
                    const value = data.value || 0;
                    const objectId = data.objectId || '';

                    let html = `<div style="max-width: 400px; word-break: break-all;">
                        <strong>${escapeHtml(name)}</strong><br/>
                        Retained Size: <b>${formatBytes(value)}</b>`;
                    if (objectId) {
                        html += `<br/>Object ID: ${escapeHtml(objectId)}`;
                    }
                    if (data.children && data.children.length > 0) {
                        html += `<br/>Contains: ${data.children.length} objects`;
                    }
                    html += `</div>`;
                    return html;
                }
            },
            series: [{
                type: 'treemap',
                data: treeData,
                width: '100%',
                height: '92%',
                top: 10,
                roam: false,
                nodeClick: 'zoomToNode',
                breadcrumb: {
                    show: true,
                    height: 24,
                    left: 'center',
                    bottom: 5,
                    itemStyle: {
                        color: '#6366f1',
                        borderColor: '#4f46e5'
                    },
                    emphasis: {
                        itemStyle: {
                            color: '#4f46e5'
                        }
                    }
                },
                label: {
                    show: true,
                    formatter: function(params) {
                        const name = params.data.name || params.name;
                        const size = formatBytes(params.data.value || 0);
                        if (name.length > 25) {
                            return name.substring(name.lastIndexOf('.') + 1).substring(0, 20) + '\n' + size;
                        }
                        return name.substring(name.lastIndexOf('.') + 1) + '\n' + size;
                    },
                    fontSize: 11,
                    color: '#fff',
                    textShadowBlur: 2,
                    textShadowColor: 'rgba(0,0,0,0.3)'
                },
                upperLabel: {
                    show: true,
                    height: 22,
                    color: '#fff',
                    fontSize: 12,
                    fontWeight: 'bold',
                    formatter: function(params) {
                        return params.name + ' (' + formatBytes(params.value) + ')';
                    }
                },
                itemStyle: {
                    borderColor: 'rgba(255,255,255,0.8)',
                    borderWidth: 1,
                    gapWidth: 1
                },
                levels: [
                    {
                        // Package/Class level
                        itemStyle: {
                            borderColor: '#334155',
                            borderWidth: 2,
                            gapWidth: 2
                        },
                        upperLabel: {
                            show: true,
                            color: '#fff',
                            fontSize: 12,
                            fontWeight: 'bold'
                        },
                        colorSaturation: [0.35, 0.6],
                        colorMappingBy: 'value'
                    },
                    {
                        // Object level
                        colorSaturation: [0.35, 0.55],
                        itemStyle: {
                            borderColorSaturation: 0.7,
                            gapWidth: 1,
                            borderWidth: 1
                        }
                    }
                ],
                color: [
                    '#ef4444', '#f97316', '#eab308', '#22c55e', '#06b6d4',
                    '#3b82f6', '#6366f1', '#8b5cf6', '#ec4899', '#f43f5e',
                    '#14b8a6', '#84cc16', '#a855f7', '#0ea5e9', '#f59e0b'
                ]
            }]
        };
    }

    function renderChart(treeData) {
        if (!containerElement) {
            containerElement = document.getElementById('retainedTreemapChart');
        }
        if (!containerElement) {
            console.warn('[RetainedTreemap] Container not found');
            return;
        }

        // Ensure container has dimensions
        if (containerElement.clientWidth === 0 || containerElement.clientHeight === 0) {
            console.log('[RetainedTreemap] Container has no size, deferring');
            return;
        }

        // Dispose old chart
        if (chart) {
            chart.dispose();
        }

        chart = echarts.init(containerElement);
        const option = generateChartOption(treeData);
        chart.setOption(option);

        // Handle click for drill-down
        chart.on('click', function(params) {
            if (params.data && params.data.objectId) {
                drillDown(params.data.objectId, params.data.name);
            }
        });
    }

    function renderNavigation() {
        const nav = document.getElementById('retainedTreemapNav');
        if (!nav) return;

        if (navigationStack.length === 0) {
            nav.innerHTML = `
                <span class="text-xs text-gray-500 dark:text-gray-400">
                    📊 Showing retained size distribution from virtual root
                </span>`;
            return;
        }

        let html = `
            <button onclick="HeapRetainedTreemap.navigateToRoot()" class="text-xs px-2 py-1 bg-indigo-100 dark:bg-indigo-900 text-indigo-600 dark:text-indigo-300 rounded hover:bg-indigo-200 dark:hover:bg-indigo-800 transition-colors">
                ⬆ Root
            </button>`;

        navigationStack.forEach((item, idx) => {
            html += `
                <span class="text-gray-400 mx-1">→</span>
                <button onclick="HeapRetainedTreemap.navigateTo(${idx})" class="text-xs px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300 rounded hover:bg-gray-200 dark:hover:bg-gray-600 transition-colors truncate max-w-[150px]" title="${escapeHtml(item.name)}">
                    ${escapeHtml(item.name.substring(item.name.lastIndexOf('.') + 1))}
                </button>`;
        });

        nav.innerHTML = `<div class="flex items-center flex-wrap gap-1">${html}</div>`;
    }

    // ============================================
    // 导航
    // ============================================

    async function drillDown(objectId, name) {
        navigationStack.push({ objectId, name });
        currentRootId = objectId;
        await loadData();
    }

    // ============================================
    // 公共方法
    // ============================================

    function init() {
        console.log('[HeapRetainedTreemap] Initializing...');
        window.addEventListener('resize', function() {
            if (chart) chart.resize();
        });
    }

    async function load(taskId) {
        currentTaskId = taskId || '';
        currentRootId = '';
        navigationStack = [];
        await loadData();
    }

    async function loadData() {
        const container = document.getElementById('retainedTreemapChart');
        if (container && (!chart || container.clientWidth === 0)) {
            container.innerHTML = `
                <div class="flex items-center justify-center h-full">
                    <div class="text-center">
                        <div class="inline-block animate-spin rounded-full h-8 w-8 border-4 border-indigo-500 border-t-transparent"></div>
                        <p class="mt-4 text-gray-500 dark:text-gray-400">Loading retained size treemap...</p>
                    </div>
                </div>`;
        }

        try {
            const data = await fetchTreemapData(currentRootId, 300);
            if (!data || data.length === 0) {
                if (container) {
                    container.innerHTML = `
                        <div class="flex items-center justify-center h-full text-gray-500 dark:text-gray-400">
                            <p>No treemap data available for this node</p>
                        </div>`;
                }
                renderNavigation();
                return;
            }

            // Transform data for ECharts
            const chartData = data.map(group => ({
                name: group.name,
                value: group.value,
                objectId: group.object_id || '',
                children: (group.children || []).map(child => ({
                    name: child.name,
                    value: child.value,
                    objectId: child.object_id || ''
                }))
            }));

            renderNavigation();
            renderChart(chartData);
        } catch (error) {
            console.error('[RetainedTreemap] Failed to load:', error);
        }
    }

    function navigateToRoot() {
        navigationStack = [];
        currentRootId = '';
        loadData();
    }

    function navigateTo(index) {
        navigationStack = navigationStack.slice(0, index + 1);
        const target = navigationStack[navigationStack.length - 1];
        currentRootId = target ? target.objectId : '';
        loadData();
    }

    function resize() {
        if (chart) {
            chart.resize();
        }
    }

    function refresh() {
        navigateToRoot();
    }

    function destroy() {
        if (chart) {
            chart.dispose();
            chart = null;
        }
    }

    // ============================================
    // 模块注册
    // ============================================

    const module = {
        init,
        load,
        resize,
        refresh,
        destroy,
        navigateToRoot,
        navigateTo,
        drillDown
    };

    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('retainedTreemap', module);
    }

    return module;
})();

window.HeapRetainedTreemap = HeapRetainedTreemap;
