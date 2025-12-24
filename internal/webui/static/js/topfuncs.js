/**
 * Top Functions Panel Module
 * Handles table/chart view switching, bar chart visualization, and thread group distribution
 */

const TopFuncsPanel = (function() {
    // Private state
    let currentView = 'table';
    let chartInstance = null;
    let threadPieInstance = null;
    let threadBarInstance = null;
    let funcsData = [];
    let threadGroupsData = [];

    // Color palette for charts - theme-aware colors
    // Light mode: vibrant colors that work well on white backgrounds
    // Dark mode: brighter, more saturated colors for dark backgrounds
    function getThemeColors() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        
        if (isDark) {
            // Dark mode: brighter, more vibrant colors
            return [
                '#818cf8', '#a78bfa', '#f472b6', '#fb7185', '#60a5fa',
                '#22d3ee', '#4ade80', '#34d399', '#fb923c', '#fbbf24',
                '#38bdf8', '#c084fc', '#a5b4fc', '#fda4af', '#67e8f9',
                '#86efac', '#5eead4', '#fdba74', '#fcd34d', '#f0abfc'
            ];
        }
        
        // Light mode: original colors
        if (typeof ThemeManager !== 'undefined') {
            const primary = ThemeManager.getColorHex('primary') || '#667eea';
            const secondary = ThemeManager.getColorHex('secondary') || '#764ba2';
            return [
                primary, secondary, '#f093fb', '#f5576c', '#4facfe',
                '#00f2fe', '#43e97b', '#38f9d7', '#fa709a', '#fee140',
                '#30cfd0', '#6366f1', '#a8edea', '#fed6e3', '#5ee7df',
                '#b490ca', '#8b5cf6', '#4389a2', '#93a5cf', '#e4efe9'
            ];
        }
        return [
            '#667eea', '#764ba2', '#f093fb', '#f5576c', '#4facfe',
            '#00f2fe', '#43e97b', '#38f9d7', '#fa709a', '#fee140',
            '#30cfd0', '#6366f1', '#a8edea', '#fed6e3', '#5ee7df',
            '#b490ca', '#8b5cf6', '#4389a2', '#93a5cf', '#e4efe9'
        ];
    }

    // Thread group specific colors (more distinct)
    // These colors are optimized for both light and dark modes
    function getThreadColors() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        
        if (isDark) {
            // Dark mode: brighter thread colors
            return [
                '#60a5fa', '#4ade80', '#fbbf24', '#f87171', '#22d3ee',
                '#34d399', '#fb923c', '#c084fc', '#f472b6', '#38bdf8',
                '#a3a3a3', '#78716c', '#d4d4d8', '#facc15', '#fca5a5',
                '#86efac', '#7dd3fc', '#d8b4fe', '#f9a8d4', '#fde047'
            ];
        }
        
        // Light mode: original thread colors
        return [
            '#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de',
            '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc', '#48b8d0',
            '#6e7074', '#546570', '#c4ccd3', '#f9c846', '#ff7875',
            '#95de64', '#69c0ff', '#b37feb', '#ff85c0', '#ffd666'
        ];
    }
    
    // Get theme-aware text colors for ECharts
    function getChartTextColor() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        return isDark ? '#e5e7eb' : '#333';
    }
    
    function getChartSecondaryTextColor() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        return isDark ? '#9ca3af' : '#666';
    }
    
    function getChartAxisLineColor() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        return isDark ? '#4b5563' : '#e0e0e0';
    }
    
    function getChartSplitLineColor() {
        const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
        return isDark ? '#374151' : '#f0f0f0';
    }

    // Get color for index
    function getColor(index, useThreadColors = false) {
        const colors = useThreadColors ? getThreadColors() : getThemeColors();
        return colors[index % colors.length];
    }

    // Truncate function name for display - keep more context
    function truncateName(name, maxLen = 80) {
        if (!name) return '';
        // Strip allocation marker for cleaner display
        const cleanName = Utils.stripAllocationMarker ? Utils.stripAllocationMarker(name) : name;
        
        // For Java methods, try to preserve class.method format
        const parts = cleanName.split(/[./]/);
        if (parts.length >= 2) {
            const method = parts[parts.length - 1];
            const className = parts[parts.length - 2];
            let shortName = className + '.' + method;
            
            if (shortName.length > maxLen) {
                shortName = method;
            }
            
            if (shortName.length > maxLen) {
                return shortName.substring(0, maxLen - 3) + '...';
            }
            return shortName;
        }
        
        if (cleanName.length > maxLen) {
            return cleanName.substring(0, maxLen - 3) + '...';
        }
        return cleanName;
    }
    
    // Calculate optimal left margin based on label lengths
    function calculateLeftMargin(names) {
        if (!names || names.length === 0) return 200;
        
        const charWidth = 6.5;
        const maxLabelLen = Math.max(...names.map(n => n.length));
        const estimatedWidth = Math.min(maxLabelLen * charWidth, 350);
        
        return Math.max(180, estimatedWidth + 20);
    }

    // Adjust color brightness
    function adjustColor(color, percent) {
        const num = parseInt(color.replace('#', ''), 16);
        const amt = Math.round(2.55 * percent);
        const R = Math.min(255, (num >> 16) + amt);
        const G = Math.min(255, ((num >> 8) & 0x00FF) + amt);
        const B = Math.min(255, (num & 0x0000FF) + amt);
        return '#' + (0x1000000 + R * 0x10000 + G * 0x100 + B).toString(16).slice(1);
    }

    // Create bar chart using ECharts
    function createBarChart(data, showOthers = true) {
        const container = document.getElementById('topFuncsBarChart');
        if (!container) return;

        if (chartInstance) {
            chartInstance.dispose();
        }

        const countSelect = document.getElementById('topFuncsChartCount');
        const displayCount = countSelect ? parseInt(countSelect.value) : 20;

        let chartData = data.slice(0, displayCount);
        let othersValue = 0;

        if (showOthers && data.length > displayCount) {
            othersValue = data.slice(displayCount).reduce((sum, f) => sum + f.self, 0);
        }

        chartData = chartData.slice().reverse();

        const names = chartData.map(f => truncateName(f.name, 60));
        const values = chartData.map(f => f.self);
        const fullNames = chartData.map(f => f.name);

        if (showOthers && othersValue > 0) {
            names.unshift('Others (' + (data.length - displayCount) + ' functions)');
            values.unshift(parseFloat(othersValue.toFixed(2)));
            fullNames.unshift('__others__');
        }

        const barColors = names.map((_, i) => {
            if (fullNames[i] === '__others__') {
                const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
                return isDark ? '#6b7280' : '#9ca3af';
            }
            return getColor(names.length - 1 - i);
        });

        const leftMargin = calculateLeftMargin(names);
        
        // Get theme-aware colors
        const textColor = getChartTextColor();
        const secondaryTextColor = getChartSecondaryTextColor();
        const axisLineColor = getChartAxisLineColor();
        const splitLineColor = getChartSplitLineColor();

        chartInstance = echarts.init(container);

        const option = {
            tooltip: {
                trigger: 'axis',
                axisPointer: { type: 'shadow' },
                confine: true,
                formatter: function(params) {
                    const idx = params[0].dataIndex;
                    const fullName = fullNames[idx];
                    const value = params[0].value;
                    if (fullName === '__others__') {
                        return `<div style="max-width: 500px;">
                            <strong>Others</strong><br/>
                            <span style="color: #888;">${data.length - displayCount} remaining functions</span><br/>
                            Combined: <strong>${value.toFixed(2)}%</strong>
                        </div>`;
                    }
                    return `<div style="max-width: 600px; word-wrap: break-word;">
                        <strong style="font-family: monospace; font-size: 11px;">${Utils.escapeHtml(fullName)}</strong><br/>
                        Self: <strong>${value.toFixed(2)}%</strong>
                    </div>`;
                }
            },
            grid: {
                left: leftMargin,
                right: 80,
                bottom: 40,
                top: 20
            },
            xAxis: {
                type: 'value',
                name: 'Self %',
                nameLocation: 'end',
                nameTextStyle: { fontSize: 11, color: secondaryTextColor },
                axisLabel: { formatter: '{value}%', fontSize: 10, color: secondaryTextColor },
                splitLine: { lineStyle: { color: splitLineColor } }
            },
            yAxis: {
                type: 'category',
                data: names,
                axisLabel: {
                    fontSize: 11,
                    color: textColor,
                    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
                    width: leftMargin - 20,
                    overflow: 'truncate',
                    ellipsis: '...'
                },
                axisTick: { show: false },
                axisLine: { lineStyle: { color: axisLineColor } }
            },
            series: [{
                name: 'Self %',
                type: 'bar',
                data: values.map((v, i) => ({
                    value: v,
                    itemStyle: {
                        color: new echarts.graphic.LinearGradient(0, 0, 1, 0, [
                            { offset: 0, color: barColors[i] },
                            { offset: 1, color: adjustColor(barColors[i], 20) }
                        ]),
                        borderRadius: [0, 4, 4, 0]
                    }
                })),
                barWidth: '60%',
                label: {
                    show: true,
                    position: 'right',
                    formatter: '{c}%',
                    fontSize: 10,
                    color: secondaryTextColor
                },
                emphasis: {
                    itemStyle: {
                        shadowBlur: 10,
                        shadowOffsetX: 0,
                        shadowColor: 'rgba(0, 0, 0, 0.2)'
                    }
                }
            }]
        };

        chartInstance.setOption(option);

        chartInstance.on('click', function(params) {
            const idx = params.dataIndex;
            const fullName = fullNames[idx];
            if (fullName && fullName !== '__others__') {
                App.searchInFlameGraph(fullName);
            }
        });
    }

    // Create thread group pie chart
    function createThreadPieChart(data, showOthers = true) {
        const container = document.getElementById('topFuncsThreadPie');
        if (!container) return;

        if (threadPieInstance) {
            threadPieInstance.dispose();
        }

        const countSelect = document.getElementById('topFuncsThreadCount');
        const displayCount = countSelect ? parseInt(countSelect.value) : 10;

        let chartData = data.slice(0, displayCount);
        let othersValue = 0;
        let othersCount = 0;

        if (showOthers && data.length > displayCount) {
            const remaining = data.slice(displayCount);
            othersValue = remaining.reduce((sum, g) => sum + g.percentage, 0);
            othersCount = remaining.length;
        }

        const pieData = chartData.map((g, i) => ({
            name: g.group_name,
            value: parseFloat(g.percentage.toFixed(2)),
            threadCount: g.thread_count,
            totalSamples: g.total_samples,
            itemStyle: { color: getColor(i, true) }
        }));

        if (showOthers && othersValue > 0) {
            const isDark = document.documentElement.getAttribute('data-theme') === 'dark';
            pieData.push({
                name: `Others (${othersCount} groups)`,
                value: parseFloat(othersValue.toFixed(2)),
                threadCount: data.slice(displayCount).reduce((sum, g) => sum + g.thread_count, 0),
                totalSamples: data.slice(displayCount).reduce((sum, g) => sum + g.total_samples, 0),
                itemStyle: { color: isDark ? '#6b7280' : '#9ca3af' }
            });
        }

        threadPieInstance = echarts.init(container);
        
        // Get theme-aware colors
        const textColor = getChartTextColor();
        const secondaryTextColor = getChartSecondaryTextColor();

        const option = {
            title: {
                text: 'Thread Group Distribution',
                left: 'center',
                top: 10,
                textStyle: { fontSize: 14, fontWeight: 'bold', color: textColor }
            },
            tooltip: {
                trigger: 'item',
                formatter: function(params) {
                    return `<div style="max-width: 300px;">
                        <strong>${Utils.escapeHtml(params.name)}</strong><br/>
                        Percentage: <strong>${params.value.toFixed(2)}%</strong><br/>
                        Threads: <strong>${params.data.threadCount}</strong><br/>
                        Samples: <strong>${params.data.totalSamples.toLocaleString()}</strong>
                    </div>`;
                }
            },
            legend: {
                type: 'scroll',
                orient: 'vertical',
                right: 10,
                top: 50,
                bottom: 20,
                textStyle: { fontSize: 10, color: secondaryTextColor },
                formatter: function(name) {
                    return name.length > 20 ? name.substring(0, 18) + '...' : name;
                }
            },
            series: [{
                name: 'Thread Groups',
                type: 'pie',
                radius: ['35%', '65%'],
                center: ['40%', '55%'],
                avoidLabelOverlap: true,
                itemStyle: {
                    borderRadius: 6,
                    borderColor: document.documentElement.getAttribute('data-theme') === 'dark' ? '#1f2937' : '#fff',
                    borderWidth: 2
                },
                label: {
                    show: true,
                    formatter: '{b}: {d}%',
                    fontSize: 10,
                    color: secondaryTextColor
                },
                labelLine: {
                    length: 10,
                    length2: 8
                },
                emphasis: {
                    label: {
                        show: true,
                        fontSize: 12,
                        fontWeight: 'bold'
                    },
                    itemStyle: {
                        shadowBlur: 10,
                        shadowOffsetX: 0,
                        shadowColor: 'rgba(0, 0, 0, 0.3)'
                    }
                },
                data: pieData
            }]
        };

        threadPieInstance.setOption(option);

        // Click handler - filter threads by group
        threadPieInstance.on('click', function(params) {
            if (params.name && !params.name.startsWith('Others')) {
                TopFuncsPanel.filterThreadGroup(params.name);
            }
        });
    }

    // Create thread group bar chart
    function createThreadBarChart(data, showOthers = true) {
        const container = document.getElementById('topFuncsThreadBar');
        if (!container) return;

        if (threadBarInstance) {
            threadBarInstance.dispose();
        }

        const countSelect = document.getElementById('topFuncsThreadCount');
        const displayCount = countSelect ? parseInt(countSelect.value) : 10;

        let chartData = data.slice(0, displayCount);
        
        // Reverse for horizontal bar (bottom to top)
        chartData = chartData.slice().reverse();

        const names = chartData.map(g => g.group_name.length > 25 ? g.group_name.substring(0, 22) + '...' : g.group_name);
        const percentages = chartData.map(g => parseFloat(g.percentage.toFixed(2)));
        const threadCounts = chartData.map(g => g.thread_count);
        const fullNames = chartData.map(g => g.group_name);

        threadBarInstance = echarts.init(container);
        
        // Get theme-aware colors
        const textColor = getChartTextColor();
        const secondaryTextColor = getChartSecondaryTextColor();
        const axisLineColor = getChartAxisLineColor();
        const splitLineColor = getChartSplitLineColor();

        const option = {
            title: {
                text: 'Top Thread Groups by CPU',
                left: 'center',
                top: 10,
                textStyle: { fontSize: 14, fontWeight: 'bold', color: textColor }
            },
            tooltip: {
                trigger: 'axis',
                axisPointer: { type: 'shadow' },
                formatter: function(params) {
                    const idx = params[0].dataIndex;
                    const fullName = fullNames[idx];
                    const pct = params[0].value;
                    const threads = threadCounts[idx];
                    return `<div style="max-width: 300px;">
                        <strong>${Utils.escapeHtml(fullName)}</strong><br/>
                        CPU: <strong>${pct.toFixed(2)}%</strong><br/>
                        Threads: <strong>${threads}</strong>
                    </div>`;
                }
            },
            grid: {
                left: 140,
                right: 60,
                bottom: 30,
                top: 50
            },
            xAxis: {
                type: 'value',
                name: 'CPU %',
                nameLocation: 'end',
                nameTextStyle: { fontSize: 10, color: secondaryTextColor },
                axisLabel: { formatter: '{value}%', fontSize: 10, color: secondaryTextColor },
                splitLine: { lineStyle: { color: splitLineColor } }
            },
            yAxis: {
                type: 'category',
                data: names,
                axisLabel: {
                    fontSize: 10,
                    color: textColor,
                    width: 120,
                    overflow: 'truncate',
                    ellipsis: '...'
                },
                axisTick: { show: false },
                axisLine: { lineStyle: { color: axisLineColor } }
            },
            series: [{
                name: 'CPU %',
                type: 'bar',
                data: percentages.map((v, i) => ({
                    value: v,
                    itemStyle: {
                        color: new echarts.graphic.LinearGradient(0, 0, 1, 0, [
                            { offset: 0, color: getColor(chartData.length - 1 - i, true) },
                            { offset: 1, color: adjustColor(getColor(chartData.length - 1 - i, true), 20) }
                        ]),
                        borderRadius: [0, 4, 4, 0]
                    }
                })),
                barWidth: '60%',
                label: {
                    show: true,
                    position: 'right',
                    formatter: function(params) {
                        return params.value.toFixed(1) + '%';
                    },
                    fontSize: 10,
                    color: secondaryTextColor
                },
                emphasis: {
                    itemStyle: {
                        shadowBlur: 10,
                        shadowColor: 'rgba(0, 0, 0, 0.2)'
                    }
                }
            }]
        };

        threadBarInstance.setOption(option);

        // Click handler - filter threads by group
        threadBarInstance.on('click', function(params) {
            const idx = params.dataIndex;
            const fullName = fullNames[idx];
            if (fullName) {
                TopFuncsPanel.filterThreadGroup(fullName);
            }
        });
    }

    // Render thread group details
    function renderThreadDetails(data) {
        const container = document.getElementById('topFuncsThreadDetails');
        if (!container) return;

        const countSelect = document.getElementById('topFuncsThreadCount');
        const displayCount = countSelect ? parseInt(countSelect.value) : 10;
        const displayData = data.slice(0, displayCount);

        container.innerHTML = displayData.map((g, i) => `
            <div class="flex items-center gap-2 p-2 bg-theme-card rounded border border-theme cursor-pointer hover:bg-theme-hover transition-colors"
                 onclick="TopFuncsPanel.filterThreadGroup('${Utils.escapeHtml(g.group_name).replace(/'/g, "\\'")}')"
                 title="Click to filter threads by this group">
                <div class="w-3 h-3 rounded-full flex-shrink-0" style="background-color: ${getColor(i, true)}"></div>
                <div class="flex-1 min-w-0">
                    <div class="font-medium text-theme-base truncate" title="${Utils.escapeHtml(g.group_name)}">${Utils.escapeHtml(g.group_name)}</div>
                    <div class="text-theme-secondary">${g.thread_count} threads · ${g.percentage.toFixed(1)}%</div>
                </div>
            </div>
        `).join('');
    }

    // Update button states
    function updateButtonStates(activeView) {
        const tableBtn = document.getElementById('topFuncsViewTable');
        const chartBtn = document.getElementById('topFuncsViewChart');
        const threadsBtn = document.getElementById('topFuncsViewThreads');
        
        const buttons = [tableBtn, chartBtn, threadsBtn];
        const activeClass = ['bg-primary', 'text-white'];
        const inactiveClass = ['bg-gray-200', 'text-gray-700', 'hover:bg-gray-300'];
        
        buttons.forEach(btn => {
            if (!btn) return;
            btn.classList.remove(...activeClass, ...inactiveClass);
            btn.classList.add(...inactiveClass);
        });
        
        let activeBtn = null;
        if (activeView === 'table') activeBtn = tableBtn;
        else if (activeView === 'chart') activeBtn = chartBtn;
        else if (activeView === 'threads') activeBtn = threadsBtn;
        
        if (activeBtn) {
            activeBtn.classList.remove(...inactiveClass);
            activeBtn.classList.add(...activeClass);
        }
    }

    // Resize handler
    function handleResize() {
        if (chartInstance) chartInstance.resize();
        if (threadPieInstance) threadPieInstance.resize();
        if (threadBarInstance) threadBarInstance.resize();
    }

    // Add resize listener
    window.addEventListener('resize', handleResize);

    // Public API
    return {
        // Set view mode
        setView: function(view) {
            // Check if threads view is allowed (only for Java cpu/alloc analysis)
            if (view === 'threads') {
                const analysisType = typeof App !== 'undefined' && App.getAnalysisType ? App.getAnalysisType() : null;
                if (analysisType === 'pprof-all') {
                    console.log('[TopFuncsPanel] Threads view not available in pprof mode, switching to table');
                    view = 'table';
                }
            }
            
            currentView = view;
            const tableView = document.getElementById('topFuncsTableView');
            const chartView = document.getElementById('topFuncsChartView');
            const threadsView = document.getElementById('topFuncsThreadsView');

            // Hide all views
            if (tableView) tableView.classList.add('hidden');
            if (chartView) chartView.classList.add('hidden');
            if (threadsView) threadsView.classList.add('hidden');

            // Show selected view
            if (view === 'table' && tableView) {
                tableView.classList.remove('hidden');
            } else if (view === 'chart' && chartView) {
                chartView.classList.remove('hidden');
                if (funcsData.length > 0) {
                    setTimeout(() => this.updateChart(), 50);
                }
            } else if (view === 'threads' && threadsView) {
                threadsView.classList.remove('hidden');
                if (threadGroupsData.length > 0) {
                    setTimeout(() => this.updateThreadChart(), 50);
                } else {
                    // Try to load thread data
                    this.loadThreadGroups();
                }
            }

            updateButtonStates(view);
        },

        // Update functions bar chart
        updateChart: function() {
            if (funcsData.length === 0) return;
            
            const showOthers = document.getElementById('topFuncsChartShowOthers');
            createBarChart(funcsData, showOthers ? showOthers.checked : true);
        },

        // Update thread group charts
        updateThreadChart: function() {
            if (threadGroupsData.length === 0) {
                this.loadThreadGroups();
                return;
            }
            
            const showOthers = document.getElementById('topFuncsThreadShowOthers');
            const show = showOthers ? showOthers.checked : true;
            
            createThreadPieChart(threadGroupsData, show);
            createThreadBarChart(threadGroupsData, show);
            renderThreadDetails(threadGroupsData);
        },

        // Load thread groups from ThreadsPanel or API
        loadThreadGroups: async function() {
            // Try to get from ThreadsPanel module first
            if (typeof ThreadsPanel !== 'undefined') {
                const state = ThreadsPanel.getState();
                if (state && state.threadGroups && state.threadGroups.length > 0) {
                    threadGroupsData = state.threadGroups;
                    this.updateThreadChart();
                    return;
                }
            }

            // Fallback: fetch from API
            try {
                const taskId = App.getCurrentTask();
                if (!taskId) {
                    this.showThreadLoadError('No task selected');
                    return;
                }

                // Determine the correct API type based on analysis type
                const analysisType = App.getAnalysisType();
                const apiType = analysisType === 'alloc' ? 'memory' : 'cpu';
                console.log('Fetching thread data for task:', taskId, 'analysisType:', analysisType, 'apiType:', apiType);
                const response = await fetch(`/api/flamegraph?type=${apiType}&task=${taskId}`);
                if (!response.ok) throw new Error('Failed to fetch: ' + response.status);
                
                const data = await response.json();
                
                // Debug: log the full response structure
                console.log('API response:', {
                    hasThreadAnalysis: !!data.thread_analysis,
                    threadGroupsLength: data.thread_analysis?.thread_groups?.length,
                    threadsLength: data.thread_analysis?.threads?.length,
                    totalSamples: data.total_samples
                });
                
                if (data.thread_analysis && data.thread_analysis.thread_groups && data.thread_analysis.thread_groups.length > 0) {
                    // Map API field names to expected format
                    // API returns: { name, thread_count, total_samples, percentage, top_thread }
                    // We need: { group_name, thread_count, total_samples, percentage, top_thread }
                    threadGroupsData = data.thread_analysis.thread_groups.map(g => ({
                        group_name: g.name || g.group_name,
                        thread_count: g.thread_count,
                        total_samples: g.total_samples,
                        percentage: g.percentage,
                        top_thread: g.top_thread
                    }));
                    console.log('Using thread_groups from API:', threadGroupsData.length, 'groups');
                } else if (data.thread_analysis && data.thread_analysis.threads && data.thread_analysis.threads.length > 0) {
                    // Compute thread groups from threads
                    const totalSamples = data.total_samples || 0;
                    console.log('Computing thread groups from', data.thread_analysis.threads.length, 'threads, totalSamples:', totalSamples);
                    threadGroupsData = this.computeThreadGroups(data.thread_analysis.threads, totalSamples);
                    console.log('Computed thread groups from threads:', threadGroupsData.length, 'groups');
                } else {
                    console.warn('No thread data in API response:', {
                        thread_analysis: data.thread_analysis,
                        thread_groups: data.thread_analysis?.thread_groups,
                        threads: data.thread_analysis?.threads
                    });
                    this.showThreadLoadError('No thread data available');
                    return;
                }
                
                // Check if we got any groups
                if (!threadGroupsData || threadGroupsData.length === 0) {
                    console.warn('Thread groups data is empty after processing');
                    this.showThreadLoadError('No thread groups found');
                    return;
                }
                
                this.updateThreadChart();
            } catch (error) {
                console.error('Failed to load thread groups:', error);
                this.showThreadLoadError('Failed to load thread data');
            }
        },

        // Show error message in thread view
        showThreadLoadError: function(message) {
            const pieContainer = document.getElementById('topFuncsThreadPie');
            const barContainer = document.getElementById('topFuncsThreadBar');
            const detailsContainer = document.getElementById('topFuncsThreadDetails');
            
            const errorHtml = `<div class="flex items-center justify-center h-full text-gray-500">${Utils.escapeHtml(message)}</div>`;
            
            if (pieContainer) pieContainer.innerHTML = errorHtml;
            if (barContainer) barContainer.innerHTML = errorHtml;
            if (detailsContainer) detailsContainer.innerHTML = '';
        },

        // Compute thread groups from threads
        computeThreadGroups: function(threads, totalSamples) {
            console.log('computeThreadGroups called with', threads.length, 'threads, totalSamples:', totalSamples);
            
            const groupMap = new Map();

            for (const thread of threads) {
                // Support both 'name' (from API) and 'thread_name' field names
                const threadName = thread.name || thread.thread_name || 'unknown';
                const group = thread.group || this.extractThreadGroup(threadName);
                
                if (!groupMap.has(group)) {
                    groupMap.set(group, {
                        group_name: group,
                        thread_count: 0,
                        total_samples: 0,
                        percentage: 0
                    });
                }

                const g = groupMap.get(group);
                g.thread_count++;
                g.total_samples += thread.samples || 0;
            }

            const groups = Array.from(groupMap.values());
            for (const g of groups) {
                g.percentage = totalSamples > 0 ? (g.total_samples / totalSamples) * 100 : 0;
            }

            groups.sort((a, b) => b.total_samples - a.total_samples);
            
            console.log('Computed', groups.length, 'thread groups:', groups.slice(0, 3));
            return groups;
        },

        // Extract thread group from thread name
        extractThreadGroup: function(name) {
            if (!name) return 'unknown';
            
            const patterns = [
                /^(.*?)-\d+$/,
                /^(.*?)_\d+$/,
                /^(.*?)\[\d+\]$/,
                /^(.*?)#\d+$/,
                /^pool-\d+-(.*?)$/,
            ];

            for (const pattern of patterns) {
                const match = name.match(pattern);
                if (match) {
                    return match[1];
                }
            }

            return name;
        },

        // Set functions data (called from renderTopFunctions)
        setData: function(data) {
            funcsData = data;
            if (currentView === 'chart') {
                this.updateChart();
            }
        },

        // Set thread groups data
        setThreadGroupsData: function(data) {
            threadGroupsData = data;
            if (currentView === 'threads') {
                this.updateThreadChart();
            }
        },

        // Filter thread group (navigate to threads panel)
        filterThreadGroup: function(groupName) {
            if (typeof ThreadsPanel !== 'undefined' && ThreadsPanel.filterByGroup) {
                ThreadsPanel.filterByGroup(groupName);
                App.showPanel('threads');
            }
        },

        // Search thread group in flame graph
        searchThreadGroupInFlameGraph: function(groupName) {
            if (!groupName) return;
            
            // Navigate to flame graph panel
            App.showPanel('flamegraph');
            
            // Wait for panel to be visible, then search
            setTimeout(() => {
                if (typeof FlameGraph !== 'undefined' && FlameGraph.search) {
                    // Set search input value
                    const searchInput = document.getElementById('searchInput');
                    if (searchInput) {
                        searchInput.value = groupName;
                    }
                    // Trigger search
                    FlameGraph.search(groupName);
                }
            }, 100);
        },

        // Get current data
        getData: function() {
            return funcsData;
        },

        // Get thread groups data
        getThreadGroupsData: function() {
            return threadGroupsData;
        },

        // Get current view
        getView: function() {
            return currentView;
        },

        // Resize charts
        resize: handleResize,
        
        // Refresh charts (called on theme change)
        refresh: function() {
            if (currentView === 'chart' && funcsData.length > 0) {
                this.updateChart();
            } else if (currentView === 'threads' && threadGroupsData.length > 0) {
                this.updateThreadChart();
            }
        },

        // Reset/clear all cached data (called when task changes)
        reset: function() {
            funcsData = [];
            threadGroupsData = [];
            // Dispose chart instances
            if (chartInstance) {
                chartInstance.dispose();
                chartInstance = null;
            }
            if (threadPieInstance) {
                threadPieInstance.dispose();
                threadPieInstance = null;
            }
            if (threadBarInstance) {
                threadBarInstance.dispose();
                threadBarInstance = null;
            }
            // Reset view to table (safe default for all analysis types)
            currentView = 'table';
            this.setView('table');
            console.log('[TopFuncsPanel] Data reset, view reset to table');
        },

        // Force reload thread groups data
        reloadThreadGroups: function() {
            threadGroupsData = [];
            this.loadThreadGroups();
        }
    };
})();

// Export for global access
window.TopFuncsPanel = TopFuncsPanel;

// Listen for theme changes to refresh charts
if (typeof ThemeManager !== 'undefined') {
    ThemeManager.onChange(function(themeId) {
        // Refresh charts when theme changes
        setTimeout(() => TopFuncsPanel.refresh(), 100);
    });
}
