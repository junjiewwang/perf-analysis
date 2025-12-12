/**
 * Heap Analysis Module
 * Handles heap treemap, histogram, and reference graph rendering
 */

const HeapAnalysis = (function() {
    // Private state
    let treemapChart = null;
    let classData = [];
    let viewMode = 'flat';
    let referenceGraphs = {};
    let businessRetainers = {};
    let gcRootPaths = {};
    let refGraphSvg = null;
    let refGraphZoom = null;

    // Public API
    return {
        init: function() {
            // Initialize event listeners
            const searchInput = document.getElementById('heapClassSearch');
            if (searchInput) {
                searchInput.addEventListener('keyup', () => this.filterClasses());
            }
        },

        renderOverview(data) {
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

            // Render top classes preview
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
        },

        renderAnalysis(data) {
            const topItems = data.top_items || [];
            const heapData = data.data || {};
            const topClasses = heapData.top_classes || [];

            // Build retainer map
            const retainerMap = {};
            topClasses.forEach(cls => {
                if (cls.retainers && cls.retainers.length > 0) {
                    retainerMap[cls.class_name] = cls.retainers;
                }
                if (cls.gc_root_paths && cls.gc_root_paths.length > 0) {
                    gcRootPaths[cls.class_name] = cls.gc_root_paths;
                }
            });

            if (heapData.reference_graphs) {
                referenceGraphs = heapData.reference_graphs;
            }

            if (heapData.business_retainers) {
                businessRetainers = heapData.business_retainers;
            }

            // Store class data
            classData = topItems.map(item => {
                const classInfo = topClasses.find(c => c.class_name === item.name) || {};
                return {
                    name: item.name,
                    size: item.value,
                    percentage: item.percentage,
                    instanceCount: item.extra ? item.extra.instance_count : 0,
                    retainers: retainerMap[item.name] || [],
                    retained_size: classInfo.retained_size || 0,
                    gc_root_paths: classInfo.gc_root_paths || []
                };
            });

            // Render stats
            document.getElementById('heapTotalSize').textContent = heapData.heap_size_human || Utils.formatBytes(heapData.total_heap_size || 0);
            document.getElementById('heapTotalClasses').textContent = Utils.formatNumber(heapData.total_classes || 0);
            document.getElementById('heapTotalInstances').textContent = Utils.formatNumber(heapData.total_instances || 0);
            document.getElementById('heapFormat').textContent = heapData.format || 'Unknown';

            this.renderTreemap(topItems, heapData.total_heap_size || 0);
            this.renderHistogram(classData);
            this.initRefGraphSelector();
        },

        renderTreemap(topItems, totalSize) {
            const container = document.getElementById('heapTreemap');
            if (!container) return;

            if (treemapChart) treemapChart.dispose();
            treemapChart = echarts.init(container);

            // Group by package
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
                    packageMap.set(packageName, { name: packageName, value: 0, children: [] });
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

            const treeData = Array.from(packageMap.values())
                .sort((a, b) => b.value - a.value)
                .slice(0, 50);

            const option = {
                title: {
                    text: 'Heap Memory Distribution by Package',
                    left: 'center',
                    textStyle: { fontSize: 16, fontWeight: 600 }
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
                        itemStyle: { color: '#9b59b6', borderColor: '#8e44ad' }
                    },
                    label: {
                        show: true,
                        formatter: function(params) {
                            const name = params.data.fullName || params.name;
                            return name.length > 20 ? name.substring(0, 17) + '...' : name;
                        },
                        fontSize: 11
                    },
                    upperLabel: { show: true, height: 20, color: '#fff' },
                    itemStyle: { borderColor: '#fff', borderWidth: 1, gapWidth: 1 },
                    levels: [
                        {
                            itemStyle: { borderColor: '#555', borderWidth: 2, gapWidth: 2 },
                            upperLabel: { show: true, color: '#fff', fontSize: 12, fontWeight: 'bold' },
                            colorSaturation: [0.3, 0.6],
                            colorMappingBy: 'value'
                        },
                        {
                            colorSaturation: [0.3, 0.5],
                            itemStyle: { borderColorSaturation: 0.7, gapWidth: 1, borderWidth: 1 }
                        }
                    ],
                    color: ['#9b59b6', '#8e44ad', '#7d3c98', '#6c3483', '#5b2c6f', '#4a235a']
                }]
            };

            treemapChart.setOption(option);

            window.addEventListener('resize', () => {
                if (treemapChart) treemapChart.resize();
            });
        },

        renderHistogram(data) {
            const tbody = document.getElementById('heapClassTableBody');
            if (!tbody) return;

            const maxSize = data.length > 0 ? data[0].size : 1;

            tbody.innerHTML = data.map((cls, i) => {
                const barWidth = (cls.size / maxSize) * 100;
                const hasRetainers = cls.retainers && cls.retainers.length > 0;
                const hasGCPaths = cls.gc_root_paths && cls.gc_root_paths.length > 0;
                const hasBusinessRetainers = businessRetainers[cls.name] && businessRetainers[cls.name].length > 0;

                const retainerBadge = hasRetainers ?
                    `<button class="expand-retainers-btn" onclick="HeapAnalysis.toggleRetainers(${i})">🔗 ${cls.retainers.length} Retainers</button>` : '';
                const gcPathBadge = hasGCPaths ?
                    `<button class="expand-retainers-btn" style="margin-left: 5px; background: #00c853; border-color: #00a844; color: white;" onclick="HeapAnalysis.showGCPaths('${Utils.escapeHtml(cls.name).replace(/'/g, "\\'")}')">📍 GC Paths</button>` : '';
                const businessBadge = hasBusinessRetainers ?
                    `<button class="expand-retainers-btn" style="margin-left: 5px; background: #ff6b35; border-color: #e55a2b; color: white;" onclick="HeapAnalysis.toggleBusinessRetainers(${i})">🎯 ${businessRetainers[cls.name].length} Root Causes</button>` : '';

                let retainerSection = '';
                if (hasRetainers) {
                    retainerSection = `
                        <tr id="retainer-row-${i}" style="display: none;">
                            <td colspan="5" class="retainer-cell">
                                <div class="retainer-section">
                                    <div class="retainer-title">
                                        <span>🔗</span> Who holds ${Utils.escapeHtml(cls.name.split('.').pop())}?
                                        ${cls.retained_size ? `<span style="margin-left: 15px; font-weight: normal; color: #666;">Retained Size: ${Utils.formatBytes(cls.retained_size)}</span>` : ''}
                                    </div>
                                    <ul class="retainer-list">
                                        ${cls.retainers.map(r => `
                                            <li class="retainer-item">
                                                <div class="retainer-class">
                                                    ${r.depth && r.depth > 1 ? `<span class="depth-indicator">${r.depth}</span>` : ''}
                                                    ${Utils.escapeHtml(r.retainer_class)}${r.field_name ? `.<span class="retainer-field">${Utils.escapeHtml(r.field_name)}</span>` : ''}
                                                </div>
                                                <div class="retainer-stats">
                                                    ${r.depth ? `<span>🔢 Depth ${r.depth}</span>` : ''}
                                                    <span>📊 ${r.percentage.toFixed(1)}%</span>
                                                    <span>📦 ${Utils.formatNumber(r.retained_count)} refs</span>
                                                    <span>💾 ${Utils.formatBytes(r.retained_size)}</span>
                                                </div>
                                            </li>
                                        `).join('')}
                                    </ul>
                                </div>
                            </td>
                        </tr>
                    `;
                }

                let businessSection = '';
                if (hasBusinessRetainers) {
                    const brs = businessRetainers[cls.name];
                    businessSection = `
                        <tr id="business-row-${i}" style="display: none;">
                            <td colspan="5" class="retainer-cell">
                                <div class="retainer-section" style="background: linear-gradient(135deg, #fff5f0 0%, #ffe8e0 100%); border-color: #ff6b35;">
                                    <div class="retainer-title" style="color: #ff6b35;">
                                        <span>🎯</span> Business-Level Root Causes for ${Utils.escapeHtml(cls.name.split('.').pop())}
                                        <span style="margin-left: 15px; font-weight: normal; color: #666; font-size: 12px;">
                                            These are application-level classes (not JDK/framework internals) that hold references
                                        </span>
                                    </div>
                                    <ul class="retainer-list">
                                        ${brs.map(r => `
                                            <li class="retainer-item" style="border-left-color: #ff6b35;">
                                                <div class="retainer-class">
                                                    <span class="depth-indicator" style="background: #ff6b35;">${r.depth}</span>
                                                    ${Utils.escapeHtml(r.class_name)}
                                                    ${r.field_path && r.field_path.length > 0 ? 
                                                        `<span class="retainer-field" style="color: #ff6b35;">via ${r.field_path.join(' → ')}</span>` : ''}
                                                    ${r.is_gc_root ? `<span style="background: #00c853; color: white; padding: 2px 6px; border-radius: 3px; font-size: 10px; margin-left: 8px;">GC ROOT: ${r.gc_root_type}</span>` : ''}
                                                </div>
                                                <div class="retainer-stats">
                                                    <span>🔢 Depth ${r.depth}</span>
                                                    <span>📊 ${r.percentage.toFixed(1)}%</span>
                                                    <span>📦 ${Utils.formatNumber(r.retained_count)} refs</span>
                                                    <span>💾 ${Utils.formatBytes(r.retained_size)}</span>
                                                </div>
                                            </li>
                                        `).join('')}
                                    </ul>
                                </div>
                            </td>
                        </tr>
                    `;
                }

                return `
                    <tr id="class-row-${i}" class="${hasRetainers || hasBusinessRetainers ? 'has-retainers' : ''}">
                        <td>${i + 1}</td>
                        <td class="class-name" title="${Utils.escapeHtml(cls.name)}">${Utils.escapeHtml(cls.name)}</td>
                        <td>
                            <div class="size-bar">
                                <div class="size-bar-bg">
                                    <div class="size-bar-fill" style="width: ${barWidth}%"></div>
                                </div>
                                <span class="size-text">${Utils.formatBytes(cls.size)}</span>
                            </div>
                        </td>
                        <td class="instance-count">${Utils.formatNumber(cls.instanceCount)}</td>
                        <td>
                            ${cls.percentage.toFixed(2)}%
                            ${businessBadge}
                            ${retainerBadge}
                            ${gcPathBadge}
                        </td>
                    </tr>
                    ${businessSection}
                    ${retainerSection}
                `;
            }).join('');
        },

        toggleRetainers(idx) {
            const retainerRow = document.getElementById(`retainer-row-${idx}`);
            const classRow = document.getElementById(`class-row-${idx}`);
            if (retainerRow) {
                const isVisible = retainerRow.style.display !== 'none';
                retainerRow.style.display = isVisible ? 'none' : 'table-row';
                if (classRow) classRow.classList.toggle('heap-class-row-expanded', !isVisible);
            }
        },

        toggleBusinessRetainers(idx) {
            const businessRow = document.getElementById(`business-row-${idx}`);
            const classRow = document.getElementById(`class-row-${idx}`);
            if (businessRow) {
                const isVisible = businessRow.style.display !== 'none';
                businessRow.style.display = isVisible ? 'none' : 'table-row';
                if (classRow) classRow.classList.toggle('heap-class-row-expanded', !isVisible);
            }
        },

        showGCPaths(className) {
            App.showPanel('heaprefgraph');
            document.getElementById('refGraphClassSelect').value = className;
            this.loadRefGraph(className);
        },

        filterClasses() {
            const searchTerm = document.getElementById('heapClassSearch').value.toLowerCase();
            const filtered = classData.filter(cls => cls.name.toLowerCase().includes(searchTerm));

            if (viewMode === 'flat') {
                this.renderHistogram(filtered);
            } else {
                this.renderPackageView(filtered);
            }
        },

        clearSearch() {
            document.getElementById('heapClassSearch').value = '';
            if (viewMode === 'flat') {
                this.renderHistogram(classData);
            } else {
                this.renderPackageView(classData);
            }
        },

        setViewMode(mode) {
            viewMode = mode;
            document.getElementById('heapViewFlat').classList.toggle('active', mode === 'flat');
            document.getElementById('heapViewPackage').classList.toggle('active', mode === 'package');
            document.getElementById('heapFlatView').style.display = mode === 'flat' ? 'block' : 'none';
            document.getElementById('heapPackageView').style.display = mode === 'package' ? 'block' : 'none';

            if (mode === 'package') {
                this.renderPackageView(classData);
            }
        },

        renderPackageView(data) {
            const container = document.getElementById('heapPackageGroups');
            if (!container) return;

            const packageMap = new Map();
            data.forEach(cls => {
                const parts = cls.name.split('.');
                let packageName = 'default';
                if (parts.length > 1) {
                    parts.pop();
                    packageName = parts.join('.');
                }

                if (!packageMap.has(packageName)) {
                    packageMap.set(packageName, { totalSize: 0, totalInstances: 0, classes: [] });
                }

                const pkg = packageMap.get(packageName);
                pkg.totalSize += cls.size;
                pkg.totalInstances += cls.instanceCount;
                pkg.classes.push(cls);
            });

            const packages = Array.from(packageMap.entries()).sort((a, b) => b[1].totalSize - a[1].totalSize);

            container.innerHTML = packages.map(([pkgName, pkg], idx) => {
                const classRows = pkg.classes.map((cls, i) => {
                    const shortName = cls.name.split('.').pop();
                    return `
                        <tr>
                            <td style="padding-left: 30px;">${i + 1}</td>
                            <td class="class-name" title="${Utils.escapeHtml(cls.name)}">${Utils.escapeHtml(shortName)}</td>
                            <td>${Utils.formatBytes(cls.size)}</td>
                            <td>${Utils.formatNumber(cls.instanceCount)}</td>
                            <td>${cls.percentage.toFixed(2)}%</td>
                        </tr>
                    `;
                }).join('');

                return `
                    <div class="heap-package-group">
                        <div class="heap-package-header" onclick="HeapAnalysis.togglePackage(${idx})">
                            <span>📦 ${Utils.escapeHtml(pkgName)}</span>
                            <div class="heap-package-stats">
                                <span>Size: ${Utils.formatBytes(pkg.totalSize)}</span>
                                <span>Instances: ${Utils.formatNumber(pkg.totalInstances)}</span>
                                <span>Classes: ${pkg.classes.length}</span>
                            </div>
                        </div>
                        <div class="heap-package-content" id="pkg-content-${idx}">
                            <table class="heap-class-table">
                                <thead>
                                    <tr>
                                        <th style="width: 50px">#</th>
                                        <th>Class</th>
                                        <th style="width: 120px">Size</th>
                                        <th style="width: 100px">Instances</th>
                                        <th style="width: 80px">%</th>
                                    </tr>
                                </thead>
                                <tbody>${classRows}</tbody>
                            </table>
                        </div>
                    </div>
                `;
            }).join('');
        },

        togglePackage(idx) {
            const content = document.getElementById(`pkg-content-${idx}`);
            if (content) content.classList.toggle('expanded');
        },

        initRefGraphSelector() {
            const select = document.getElementById('refGraphClassSelect');
            if (!select) return;

            select.innerHTML = '<option value="">-- Select a class --</option>';
            classData.slice(0, 20).forEach((cls, i) => {
                const option = document.createElement('option');
                option.value = cls.name;
                option.textContent = `${i + 1}. ${cls.name} (${Utils.formatBytes(cls.size)})`;
                select.appendChild(option);
            });
        },

        loadRefGraph(className) {
            if (!className) {
                document.getElementById('heapRefGraph').innerHTML = '<div class="loading">Select a class to view its reference graph</div>';
                document.getElementById('gcPathsContainer').style.display = 'none';
                return;
            }

            if (referenceGraphs && referenceGraphs[className]) {
                this.renderRefGraph(referenceGraphs[className], className);
            } else {
                const data = classData.find(c => c.name === className);
                if (data && data.retainers && data.retainers.length > 0) {
                    const graphData = this.generateRefGraphFromRetainers(data);
                    this.renderRefGraph(graphData, className);
                } else {
                    document.getElementById('heapRefGraph').innerHTML = '<div class="loading">No reference data available for this class</div>';
                }
            }

            this.renderGCRootPaths(className);
        },

        generateRefGraphFromRetainers(data) {
            const nodes = [];
            const edges = [];
            const nodeMap = new Map();

            const targetId = 'target_' + data.name;
            nodes.push({
                id: targetId,
                class_name: data.name,
                size: data.size,
                is_gc_root: false,
                is_target: true
            });
            nodeMap.set(targetId, true);

            data.retainers.forEach((r, i) => {
                const retainerId = 'retainer_' + i + '_' + r.retainer_class;
                if (!nodeMap.has(retainerId)) {
                    nodes.push({
                        id: retainerId,
                        class_name: r.retainer_class,
                        size: r.retained_size,
                        is_gc_root: false,
                        depth: r.depth || 1
                    });
                    nodeMap.set(retainerId, true);
                }

                edges.push({
                    source: retainerId,
                    target: targetId,
                    field_name: r.field_name || ''
                });
            });

            return { nodes, edges };
        },

        renderRefGraph(graphData, className) {
            const container = document.getElementById('heapRefGraph');
            container.innerHTML = '';

            if (!graphData || !graphData.nodes || graphData.nodes.length === 0) {
                container.innerHTML = '<div class="loading">No reference graph data available</div>';
                return;
            }

            const width = container.clientWidth || 800;
            const height = container.clientHeight || 600;

            refGraphSvg = d3.select('#heapRefGraph')
                .append('svg')
                .attr('width', width)
                .attr('height', height);

            refGraphZoom = d3.zoom()
                .scaleExtent([0.1, 4])
                .on('zoom', (event) => {
                    g.attr('transform', event.transform);
                });

            refGraphSvg.call(refGraphZoom);

            const g = refGraphSvg.append('g');

            refGraphSvg.append('defs').append('marker')
                .attr('id', 'arrowhead-ref')
                .attr('viewBox', '-0 -5 10 10')
                .attr('refX', 20)
                .attr('refY', 0)
                .attr('orient', 'auto')
                .attr('markerWidth', 8)
                .attr('markerHeight', 8)
                .append('path')
                .attr('d', 'M 0,-5 L 10,0 L 0,5')
                .attr('fill', '#999');

            const nodes = graphData.nodes.map(n => ({
                ...n,
                id: n.id,
                label: Utils.getShortClassName(n.class_name)
            }));

            const nodeById = new Map(nodes.map(n => [n.id, n]));

            const links = graphData.edges.map(e => ({
                source: nodeById.get(e.source) || e.source,
                target: nodeById.get(e.target) || e.target,
                field_name: e.field_name
            })).filter(l => l.source && l.target);

            const simulation = d3.forceSimulation(nodes)
                .force('link', d3.forceLink(links).id(d => d.id).distance(120))
                .force('charge', d3.forceManyBody().strength(-400))
                .force('center', d3.forceCenter(width / 2, height / 2))
                .force('collision', d3.forceCollide().radius(50));

            const link = g.append('g')
                .attr('class', 'links')
                .selectAll('line')
                .data(links)
                .enter()
                .append('line')
                .attr('class', 'ref-graph-edge')
                .attr('marker-end', 'url(#arrowhead-ref)');

            const linkLabel = g.append('g')
                .attr('class', 'link-labels')
                .selectAll('text')
                .data(links)
                .enter()
                .append('text')
                .attr('class', 'ref-graph-edge-label')
                .text(d => d.field_name || '');

            const node = g.append('g')
                .attr('class', 'nodes')
                .selectAll('g')
                .data(nodes)
                .enter()
                .append('g')
                .attr('class', d => {
                    let cls = 'ref-graph-node';
                    if (d.is_gc_root) cls += ' gc-root';
                    if (d.is_target) cls += ' target';
                    return cls;
                })
                .call(d3.drag()
                    .on('start', (event, d) => {
                        if (!event.active) simulation.alphaTarget(0.3).restart();
                        d.fx = d.x;
                        d.fy = d.y;
                    })
                    .on('drag', (event, d) => {
                        d.fx = event.x;
                        d.fy = event.y;
                    })
                    .on('end', (event, d) => {
                        if (!event.active) simulation.alphaTarget(0);
                        d.fx = null;
                        d.fy = null;
                    }));

            node.append('circle')
                .attr('r', d => {
                    if (d.is_target) return 25;
                    if (d.is_gc_root) return 20;
                    return 15;
                })
                .attr('fill', d => {
                    if (d.is_target) return '#e040fb';
                    if (d.is_gc_root) return '#00c853';
                    return '#7c4dff';
                });

            node.filter(d => d.depth && d.depth > 1)
                .append('text')
                .attr('class', 'depth-indicator')
                .attr('dy', -20)
                .attr('text-anchor', 'middle')
                .text(d => d.depth);

            node.append('text')
                .attr('dy', 35)
                .attr('text-anchor', 'middle')
                .text(d => d.label);

            node.append('title')
                .text(d => `${d.class_name}\nSize: ${Utils.formatBytes(d.size || 0)}${d.retained_size ? '\nRetained: ' + Utils.formatBytes(d.retained_size) : ''}`);

            simulation.on('tick', () => {
                link
                    .attr('x1', d => d.source.x)
                    .attr('y1', d => d.source.y)
                    .attr('x2', d => d.target.x)
                    .attr('y2', d => d.target.y);

                linkLabel
                    .attr('x', d => (d.source.x + d.target.x) / 2)
                    .attr('y', d => (d.source.y + d.target.y) / 2);

                node.attr('transform', d => `translate(${d.x},${d.y})`);
            });
        },

        renderGCRootPaths(className) {
            const container = document.getElementById('gcPathsContainer');
            const list = document.getElementById('gcPathsList');

            const data = classData.find(c => c.name === className);
            const paths = data?.gc_root_paths || gcRootPaths[className] || [];

            if (!paths || paths.length === 0) {
                container.style.display = 'none';
                return;
            }

            container.style.display = 'block';

            list.innerHTML = paths.slice(0, 5).map((path, i) => {
                const pathNodes = path.path || [];
                const pathHtml = pathNodes.map((node, j) => {
                    let nodeClass = '';
                    if (j === 0) nodeClass = 'root';
                    else if (j === pathNodes.length - 1) nodeClass = 'target';

                    const fieldHtml = node.field_name ?
                        `<span class="gc-path-field">.${Utils.escapeHtml(node.field_name)}</span>` : '';

                    return `
                        <span class="gc-path-node ${nodeClass}">
                            ${Utils.escapeHtml(Utils.getShortClassName(node.class_name))}${fieldHtml}
                        </span>
                        ${j < pathNodes.length - 1 ? '<span class="gc-path-arrow">→</span>' : ''}
                    `;
                }).join('');

                return `
                    <div class="gc-path-item">
                        <strong style="margin-right: 10px;">[${path.root_type || 'ROOT'}]</strong>
                        ${pathHtml}
                        <span style="margin-left: auto; color: #666; font-size: 11px;">Depth: ${path.depth || pathNodes.length}</span>
                    </div>
                `;
            }).join('');
        },

        fitRefGraph() {
            if (!refGraphSvg || !refGraphZoom) return;
            const container = document.getElementById('heapRefGraph');
            const width = container.clientWidth;
            const height = container.clientHeight;

            refGraphSvg.transition()
                .duration(500)
                .call(refGraphZoom.transform, d3.zoomIdentity
                    .translate(width / 2, height / 2)
                    .scale(0.8)
                    .translate(-width / 2, -height / 2));
        },

        resetRefGraph() {
            if (!refGraphSvg || !refGraphZoom) return;
            refGraphSvg.transition()
                .duration(500)
                .call(refGraphZoom.transform, d3.zoomIdentity);
        },

        resizeTreemap() {
            if (treemapChart) treemapChart.resize();
        },

        getClassData() {
            return classData;
        },

        // Root Cause Analysis functions
        renderRootCauseAnalysis(summaryData) {
            const suggestions = summaryData.suggestions || [];
            const topClasses = summaryData.data?.top_classes || [];
            const businessSummary = summaryData.data?.business_retainers_summary || {};

            // Render leak suspects
            this.renderLeakSuspects(topClasses, suggestions);

            // Render suggestions
            this.renderSuggestions(suggestions);

            // Load detailed retainer data on demand
            this.loadDetailedRetainers();
        },

        renderLeakSuspects(topClasses, suggestions) {
            const container = document.getElementById('leakSuspectsContainer');
            if (!container) return;

            // Identify leak suspects based on suggestions and class patterns
            const leakSuspects = [];
            
            for (const cls of topClasses.slice(0, 10)) {
                const relatedSuggestion = suggestions.find(s => 
                    s.func === cls.class_name || s.suggestion?.includes(cls.class_name)
                );
                
                let risk = 'low';
                let reason = '';
                
                if (cls.percentage > 20) {
                    risk = 'high';
                    reason = `占用堆内存 ${cls.percentage.toFixed(1)}%，超过 20% 阈值`;
                } else if (cls.percentage > 10) {
                    risk = 'medium';
                    reason = `占用堆内存 ${cls.percentage.toFixed(1)}%，超过 10% 阈值`;
                } else if (cls.has_retainers && cls.instance_count > 10000) {
                    risk = 'medium';
                    reason = `实例数量过多 (${Utils.formatNumber(cls.instance_count)})，可能存在集合类泄漏`;
                }

                if (relatedSuggestion) {
                    reason = relatedSuggestion.suggestion;
                    if (cls.percentage > 10) risk = 'high';
                }

                if (risk !== 'low' || relatedSuggestion) {
                    leakSuspects.push({
                        ...cls,
                        risk,
                        reason
                    });
                }
            }

            if (leakSuspects.length === 0) {
                container.innerHTML = `
                    <div class="no-data-message">
                        <div class="icon">✅</div>
                        <div>未发现明显的内存泄漏嫌疑</div>
                    </div>
                `;
                return;
            }

            container.innerHTML = leakSuspects.map(suspect => `
                <div class="leak-suspect-card ${suspect.risk}-risk">
                    <div class="leak-suspect-header">
                        <div class="leak-suspect-class">${Utils.escapeHtml(suspect.class_name)}</div>
                        <span class="leak-suspect-risk ${suspect.risk}">${suspect.risk === 'high' ? '高风险' : suspect.risk === 'medium' ? '中风险' : '低风险'}</span>
                    </div>
                    <div class="leak-suspect-stats">
                        <span>📊 ${suspect.percentage.toFixed(2)}%</span>
                        <span>💾 ${Utils.formatBytes(suspect.total_size)}</span>
                        <span>📦 ${Utils.formatNumber(suspect.instance_count)} 实例</span>
                        ${suspect.retained_size ? `<span>🔗 Retained: ${Utils.formatBytes(suspect.retained_size)}</span>` : ''}
                    </div>
                    ${suspect.reason ? `<div class="leak-suspect-reason">💡 ${Utils.escapeHtml(suspect.reason)}</div>` : ''}
                </div>
            `).join('');
        },

        renderSuggestions(suggestions) {
            const container = document.getElementById('heapSuggestionsContainer');
            if (!container) return;

            if (!suggestions || suggestions.length === 0) {
                container.innerHTML = `
                    <div class="no-data-message">
                        <div class="icon">📝</div>
                        <div>暂无优化建议</div>
                    </div>
                `;
                return;
            }

            container.innerHTML = suggestions.map(sug => `
                <div class="suggestion-card">
                    <span class="suggestion-icon">💡</span>
                    <span class="suggestion-text">${Utils.escapeHtml(sug.suggestion)}</span>
                    ${sug.func ? `<div class="suggestion-func">📍 ${Utils.escapeHtml(sug.func)}</div>` : ''}
                </div>
            `).join('');
        },

        async loadDetailedRetainers() {
            const container = document.getElementById('businessRetainersContainer');
            if (!container) return;

            container.innerHTML = '<div class="loading">Loading detailed retainer data...</div>';

            try {
                const taskId = new URLSearchParams(window.location.search).get('task') || '';
                const response = await fetch(`/api/retainers?task=${taskId}`);
                
                if (!response.ok) {
                    throw new Error('Failed to load retainer data');
                }

                const data = await response.json();
                
                // Store detailed data
                if (data.business_retainers) {
                    businessRetainers = data.business_retainers;
                }
                if (data.reference_graphs) {
                    referenceGraphs = data.reference_graphs;
                }
                if (data.top_classes) {
                    // Update class data with detailed retainer info
                    data.top_classes.forEach(cls => {
                        const existing = classData.find(c => c.name === cls.class_name);
                        if (existing) {
                            existing.retainers = cls.retainers || [];
                            existing.gc_root_paths = cls.gc_root_paths || [];
                        }
                    });
                }

                this.renderBusinessRetainers(businessRetainers);
            } catch (error) {
                console.error('Failed to load detailed retainers:', error);
                container.innerHTML = `
                    <div class="no-data-message">
                        <div class="icon">⚠️</div>
                        <div>无法加载详细的 Retainer 数据</div>
                        <div style="font-size: 12px; margin-top: 5px;">${error.message}</div>
                    </div>
                `;
            }
        },

        renderBusinessRetainers(retainers) {
            const container = document.getElementById('businessRetainersContainer');
            if (!container) return;

            if (!retainers || Object.keys(retainers).length === 0) {
                container.innerHTML = `
                    <div class="no-data-message">
                        <div class="icon">📊</div>
                        <div>未找到业务级别的 Retainer 数据</div>
                    </div>
                `;
                return;
            }

            // Sort by total retained size
            const sortedEntries = Object.entries(retainers)
                .map(([className, items]) => ({
                    className,
                    items,
                    totalRetained: items.reduce((sum, r) => sum + r.retained_size, 0),
                    hasGCRoots: items.some(r => r.is_gc_root)
                }))
                .sort((a, b) => b.totalRetained - a.totalRetained);

            container.innerHTML = sortedEntries.map((entry, idx) => `
                <div class="business-retainer-group">
                    <div class="business-retainer-header" onclick="HeapAnalysis.toggleBusinessGroup(${idx})">
                        <div class="business-retainer-target">
                            🎯 ${Utils.escapeHtml(entry.className)}
                            ${entry.hasGCRoots ? '<span class="gc-root-badge">Contains GC Roots</span>' : ''}
                        </div>
                        <div class="business-retainer-summary">
                            <span>📦 ${entry.items.length} retainers</span>
                            <span>💾 ${Utils.formatBytes(entry.totalRetained)}</span>
                        </div>
                    </div>
                    <div class="business-retainer-content" id="business-group-${idx}">
                        ${entry.items.map(r => `
                            <div class="business-retainer-item ${r.is_gc_root ? 'gc-root' : ''}">
                                <div style="flex: 1;">
                                    <div class="business-retainer-class">
                                        <span class="depth-indicator">${r.depth}</span>
                                        ${Utils.escapeHtml(r.class_name)}
                                        ${r.is_gc_root ? `<span class="gc-root-badge">${r.gc_root_type || 'GC ROOT'}</span>` : ''}
                                    </div>
                                    ${r.field_path && r.field_path.length > 0 ? 
                                        `<div class="business-retainer-path">via ${r.field_path.join(' → ')}</div>` : ''}
                                </div>
                                <div class="business-retainer-metrics">
                                    <span>📊 ${r.percentage.toFixed(1)}%</span>
                                    <span>📦 ${Utils.formatNumber(r.retained_count)} refs</span>
                                    <span>💾 ${Utils.formatBytes(r.retained_size)}</span>
                                </div>
                            </div>
                        `).join('')}
                    </div>
                </div>
            `).join('');
        },

        toggleBusinessGroup(idx) {
            const content = document.getElementById(`business-group-${idx}`);
            if (content) {
                content.classList.toggle('expanded');
            }
        },

        filterRootCause() {
            const searchTerm = document.getElementById('rootCauseSearch')?.value?.toLowerCase() || '';
            
            if (!searchTerm) {
                this.renderBusinessRetainers(businessRetainers);
                return;
            }

            const filtered = {};
            for (const [className, items] of Object.entries(businessRetainers)) {
                if (className.toLowerCase().includes(searchTerm)) {
                    filtered[className] = items;
                } else {
                    const matchingItems = items.filter(r => 
                        r.class_name.toLowerCase().includes(searchTerm)
                    );
                    if (matchingItems.length > 0) {
                        filtered[className] = matchingItems;
                    }
                }
            }

            this.renderBusinessRetainers(filtered);
        }
    };
})();

// Export for global access
window.HeapAnalysis = HeapAnalysis;
