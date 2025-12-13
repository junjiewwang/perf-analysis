/**
 * Heap Treemap Module
 * Treemap 可视化模块：负责堆内存分布的树状图展示
 * 
 * 职责：
 * - 渲染 ECharts Treemap 图表
 * - 处理包级别的数据聚合
 * - 响应窗口大小变化
 */

const HeapTreemap = (function() {
    'use strict';

    // ============================================
    // 私有状态
    // ============================================
    
    let treemapChart = null;
    let containerElement = null;

    // ============================================
    // 私有方法
    // ============================================
    
    /**
     * 按包名聚合数据
     * @param {Array} topItems - 顶级项目数据
     * @returns {Array} 聚合后的树状数据
     */
    function aggregateByPackage(topItems) {
        const packageMap = new Map();
        
        topItems.forEach(item => {
            const name = item.name;
            const parts = name.split('.');
            let packageName = 'default';
            let className = name;

            if (parts.length > 1) {
                className = parts.pop();
                packageName = parts.join('.');
            }

            if (!packageMap.has(packageName)) {
                packageMap.set(packageName, { 
                    name: packageName, 
                    value: 0, 
                    children: [] 
                });
            }

            const pkg = packageMap.get(packageName);
            pkg.value += item.value;
            pkg.children.push({
                name: className,
                value: item.value,
                fullName: item.name,
                percentage: item.percentage,
                instanceCount: item.extra ? item.extra.instance_count : 0
            });
        });

        return Array.from(packageMap.values())
            .sort((a, b) => b.value - a.value)
            .slice(0, 50);
    }

    /**
     * 生成 ECharts 配置
     * @param {Array} treeData - 树状数据
     * @returns {Object} ECharts 配置对象
     */
    function generateChartOption(treeData) {
        return {
            title: {
                text: 'Heap Memory Distribution by Package',
                left: 'center',
                textStyle: { 
                    fontSize: 16, 
                    fontWeight: 600 
                }
            },
            tooltip: {
                formatter: function(info) {
                    const data = info.data;
                    if (data.fullName) {
                        return `<div style="max-width: 400px; word-break: break-all;">
                            <strong>${Utils.escapeHtml(data.fullName)}</strong><br/>
                            Size: ${Utils.formatBytes(data.value)}<br/>
                            Percentage: ${data.percentage.toFixed(2)}%<br/>
                            Instances: ${Utils.formatNumber(data.instanceCount)}
                        </div>`;
                    } else {
                        return `<div style="max-width: 400px; word-break: break-all;">
                            <strong>Package: ${Utils.escapeHtml(data.name)}</strong><br/>
                            Total Size: ${Utils.formatBytes(data.value)}<br/>
                            Classes: ${data.children ? data.children.length : 0}
                        </div>`;
                    }
                }
            },
            series: [{
                type: 'treemap',
                data: treeData,
                width: '100%',
                height: '90%',
                top: 40,
                roam: 'move',
                nodeClick: 'zoomToNode',
                breadcrumb: {
                    show: true,
                    height: 22,
                    left: 'center',
                    top: 'bottom',
                    itemStyle: { 
                        color: '#9b59b6', 
                        borderColor: '#8e44ad' 
                    }
                },
                label: {
                    show: true,
                    formatter: function(params) {
                        const name = params.data.fullName || params.name;
                        return name.length > 20 ? name.substring(0, 17) + '...' : name;
                    },
                    fontSize: 11
                },
                upperLabel: { 
                    show: true, 
                    height: 20, 
                    color: '#fff' 
                },
                itemStyle: { 
                    borderColor: '#fff', 
                    borderWidth: 1, 
                    gapWidth: 1 
                },
                levels: [
                    {
                        itemStyle: { 
                            borderColor: '#555', 
                            borderWidth: 2, 
                            gapWidth: 2 
                        },
                        upperLabel: { 
                            show: true, 
                            color: '#fff', 
                            fontSize: 12, 
                            fontWeight: 'bold' 
                        },
                        colorSaturation: [0.3, 0.6],
                        colorMappingBy: 'value'
                    },
                    {
                        colorSaturation: [0.3, 0.5],
                        itemStyle: { 
                            borderColorSaturation: 0.7, 
                            gapWidth: 1, 
                            borderWidth: 1 
                        }
                    }
                ],
                color: ['#9b59b6', '#8e44ad', '#7d3c98', '#6c3483', '#5b2c6f', '#4a235a']
            }]
        };
    }

    /**
     * 处理窗口大小变化
     */
    function handleResize() {
        if (treemapChart) {
            treemapChart.resize();
        }
    }

    // ============================================
    // 公共方法
    // ============================================
    
    /**
     * 初始化模块
     */
    function init() {
        containerElement = document.getElementById('heapTreemap');
        
        // 监听窗口大小变化
        window.addEventListener('resize', handleResize);
        
        // 监听数据加载事件
        HeapCore.on('dataLoaded', function(data) {
            if (data.topItems && data.topItems.length > 0) {
                render(data.topItems, data.heapData.total_heap_size || 0);
            }
        });
    }

    /**
     * 渲染 Treemap
     * @param {Array} topItems - 顶级项目数据
     * @param {number} totalSize - 总堆大小
     */
    function render(topItems, totalSize) {
        if (!containerElement) {
            containerElement = document.getElementById('heapTreemap');
        }
        
        if (!containerElement) {
            console.warn('[HeapTreemap] Container element not found');
            return;
        }

        // 销毁旧图表
        if (treemapChart) {
            treemapChart.dispose();
        }

        // 初始化新图表
        treemapChart = echarts.init(containerElement);

        // 聚合数据
        const treeData = aggregateByPackage(topItems);

        // 生成配置并渲染
        const option = generateChartOption(treeData);
        treemapChart.setOption(option);
    }

    /**
     * 调整图表大小
     */
    function resize() {
        handleResize();
    }

    /**
     * 销毁模块
     */
    function destroy() {
        window.removeEventListener('resize', handleResize);
        
        if (treemapChart) {
            treemapChart.dispose();
            treemapChart = null;
        }
    }

    /**
     * 获取图表实例
     * @returns {Object|null} ECharts 实例
     */
    function getChart() {
        return treemapChart;
    }

    // ============================================
    // 模块注册
    // ============================================
    
    const module = {
        init,
        render,
        resize,
        destroy,
        getChart
    };

    // 自动注册到核心模块
    if (typeof HeapCore !== 'undefined') {
        HeapCore.registerModule('treemap', module);
    }

    return module;
})();

// 导出到全局
window.HeapTreemap = HeapTreemap;
