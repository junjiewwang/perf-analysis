/**
 * Main Application Module
 * Handles initialization, task loading, and panel management
 */

const App = (function() {
    // Private state
    let currentTask = '';
    let summaryData = null;
    let currentAnalysisType = 'cpu';

    // Public API
    return {
        async init() {
            // Initialize modules
            FlameGraph.init();
            CallGraph.init();
            HeapAnalysis.init();

            // Load tasks
            await this.loadTasks();
        },

        async loadTasks() {
            try {
                const tasks = await API.getTasks();
                const select = document.getElementById('taskSelect');
                select.innerHTML = '';

                if (tasks && tasks.length > 0) {
                    tasks.forEach((task, idx) => {
                        const option = document.createElement('option');
                        option.value = task.id;
                        option.textContent = task.id + (idx === 0 ? ' (latest)' : '');
                        select.appendChild(option);
                    });

                    await this.loadTask(tasks[0].id);
                } else {
                    select.innerHTML = '<option value="">No tasks found</option>';
                }
            } catch (err) {
                console.error('Failed to load tasks:', err);
                document.getElementById('taskSelect').innerHTML = '<option value="">Error loading tasks</option>';
            }
        },

        async loadTask(taskId) {
            currentTask = taskId;
            await this.loadSummary(taskId);

            if (currentAnalysisType === 'heap') {
                HeapAnalysis.renderAnalysis(summaryData);
            } else {
                await Promise.all([
                    FlameGraph.load(taskId),
                    CallGraph.load(taskId)
                ]);
            }
        },

        async loadSummary(taskId) {
            try {
                summaryData = await API.getSummary(taskId);
                this.renderSummary(summaryData);
            } catch (err) {
                console.error('Failed to load summary:', err);
            }
        },

        renderSummary(data) {
            this.detectAndSetAnalysisType(data);

            if (currentAnalysisType === 'heap') {
                HeapAnalysis.renderOverview(data);
                this.renderTaskMetadata(data.metadata);
                return;
            }

            // Stats for CPU/Allocation analysis
            document.getElementById('totalSamples').textContent = data.total_records || 0;
            document.getElementById('topFuncsCount').textContent = (data.top_items || []).length || Object.keys(data.top_funcs || {}).length;
            document.getElementById('threadsCount').textContent = (data.threads || []).length;
            document.getElementById('taskUUID').textContent = data.task_uuid || '-';

            this.renderTaskMetadata(data.metadata);

            // Top functions
            let funcs = [];
            if (data.top_items && data.top_items.length > 0) {
                funcs = data.top_items.map(item => ({ name: item.name, self: item.percentage || 0 }));
            } else if (data.top_funcs) {
                funcs = Object.entries(data.top_funcs)
                    .map(([name, val]) => ({ name, self: val.self || 0 }))
                    .sort((a, b) => b.self - a.self);
            }

            const previewBody = document.getElementById('topFuncsPreview');
            previewBody.innerHTML = funcs.slice(0, 5).map((f, i) => `
                <tr>
                    <td>${i + 1}</td>
                    <td class="func-name func-name-clickable" title="Click to copy function name" onclick="Utils.copyToClipboard('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">${Utils.escapeHtml(f.name)}</td>
                    <td>
                        <div class="percentage-bar">
                            <div class="percentage-bar-fill" style="width: ${Math.min(f.self, 100)}%"></div>
                        </div>
                        ${f.self.toFixed(2)}%
                    </td>
                    <td>
                        <button class="action-btn flame" title="Search in Flame Graph" onclick="App.searchInFlameGraph('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">🔥</button>
                        <button class="action-btn callgraph" title="Search in Call Graph" onclick="App.searchInCallGraph('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">📈</button>
                    </td>
                </tr>
            `).join('');

            const allBody = document.getElementById('topFuncsAll');
            allBody.innerHTML = funcs.map((f, i) => `
                <tr>
                    <td>${i + 1}</td>
                    <td class="func-name func-name-clickable" title="Click to copy function name" onclick="Utils.copyToClipboard('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">${Utils.escapeHtml(f.name)}</td>
                    <td>
                        <div class="percentage-bar">
                            <div class="percentage-bar-fill" style="width: ${Math.min(f.self, 100)}%"></div>
                        </div>
                        ${f.self.toFixed(2)}%
                    </td>
                    <td>
                        <button class="action-btn flame" title="Search in Flame Graph" onclick="App.searchInFlameGraph('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">🔥</button>
                        <button class="action-btn callgraph" title="Search in Call Graph" onclick="App.searchInCallGraph('${Utils.escapeHtml(f.name).replace(/'/g, "\\'")}')">📈</button>
                    </td>
                </tr>
            `).join('');

            // Threads
            const threadList = document.getElementById('threadList');
            threadList.innerHTML = (data.threads || []).map(t => `
                <li class="thread-item">
                    <span class="thread-name">${Utils.escapeHtml(t.thread_name || 'Unknown')}</span>
                    <span class="thread-samples">${t.samples} samples (${(t.percentage || 0).toFixed(2)}%)</span>
                </li>
            `).join('');
        },

        detectAndSetAnalysisType(data) {
            const taskType = data.task_type || '';
            const metadata = data.metadata || {};
            const taskTypeName = metadata.task_type_name || taskType;

            if (taskTypeName === 'java_heap' || taskType === 'java_heap' ||
                (data.data && data.data.total_heap_size !== undefined)) {
                currentAnalysisType = 'heap';
            } else {
                currentAnalysisType = 'cpu';
            }

            this.updateTabVisibility();
        },

        updateTabVisibility() {
            const cpuTabs = document.querySelectorAll('.tab.cpu-tab');
            const heapTabs = document.querySelectorAll('.tab.heap-tab');

            if (currentAnalysisType === 'heap') {
                cpuTabs.forEach(tab => tab.classList.add('hidden'));
                heapTabs.forEach(tab => tab.classList.remove('hidden'));
            } else {
                cpuTabs.forEach(tab => tab.classList.remove('hidden'));
                heapTabs.forEach(tab => tab.classList.add('hidden'));
            }
        },

        renderTaskMetadata(metadata) {
            const card = document.getElementById('taskMetadataCard');
            if (!metadata) {
                card.style.display = 'none';
                return;
            }

            card.style.display = 'block';

            const typeName = metadata.task_type_name || 'unknown';
            const typeClass = this.getTaskTypeClass(typeName);
            const typeIcon = this.getTaskTypeIcon(typeName);
            document.getElementById('metaTaskType').innerHTML = `<span class="type-badge ${typeClass}">${typeIcon} ${typeName.toUpperCase()}</span>`;

            const profilerName = metadata.profiler_name || 'unknown';
            const profilerIcon = this.getProfilerIcon(profilerName);
            document.getElementById('metaProfiler').innerHTML = `<span class="profiler-badge">${profilerIcon} ${profilerName}</span>`;

            document.getElementById('metaInputFile').textContent = metadata.input_file || '-';
            document.getElementById('metaCreatedAt').textContent = metadata.created_at ? Utils.formatDateTime(metadata.created_at) : '-';
            document.getElementById('metaAnalysisTime').textContent = metadata.analysis_time_ms ? Utils.formatDuration(metadata.analysis_time_ms) : '-';
            document.getElementById('metaTaskUUID').textContent = summaryData?.task_uuid || '-';
        },

        getTaskTypeClass(typeName) {
            const typeMap = {
                'java': 'java', 'generic': 'generic', 'pprof_mem': 'pprof',
                'memleak': 'memory', 'java_heap': 'heap', 'phys_mem': 'memory',
                'jeprof': 'memory', 'tracing': 'tracing', 'timing': 'tracing'
            };
            return typeMap[typeName] || 'generic';
        },

        getTaskTypeIcon(typeName) {
            const iconMap = {
                'java': '☕', 'generic': '🔧', 'pprof_mem': '🐹', 'memleak': '💾',
                'java_heap': '📦', 'phys_mem': '🧠', 'jeprof': '📊', 'tracing': '🔍',
                'timing': '⏱️', 'bolt': '⚡'
            };
            return iconMap[typeName] || '📊';
        },

        getProfilerIcon(profilerName) {
            const iconMap = { 'perf': '🔥', 'async_alloc': '📈', 'pprof': '🐹' };
            return iconMap[profilerName] || '📊';
        },

        showPanel(panelId) {
            // Update tabs
            document.querySelectorAll('.tab').forEach(tab => tab.classList.remove('active'));
            if (event && event.target) {
                event.target.classList.add('active');
            } else {
                // Programmatic call - find and activate the correct tab
                const tabId = `tab-${panelId}`;
                const tab = document.getElementById(tabId);
                if (tab) tab.classList.add('active');
            }

            // Update panels
            document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));

            if (panelId === 'flamegraph') {
                document.getElementById('flamegraph-panel').classList.add('active');
                if (FlameGraph.getData()) {
                    requestAnimationFrame(() => {
                        const container = document.getElementById('flamegraph');
                        if (container.clientWidth > 0) {
                            FlameGraph.render();
                        }
                    });
                }
            } else if (panelId === 'callgraph') {
                document.getElementById('callgraph-panel').classList.add('active');
            } else if (panelId === 'heapdiagnosis') {
                document.getElementById('heapdiagnosis-panel').classList.add('active');
                // Trigger diagnosis rendering if needed
                if (summaryData) {
                    HeapAnalysis.renderDiagnosis(summaryData);
                }
            } else if (panelId === 'heaptreemap') {
                document.getElementById('heaptreemap-panel').classList.add('active');
                requestAnimationFrame(() => HeapAnalysis.resizeTreemap());
            } else if (panelId === 'heapgcroots') {
                document.getElementById('heapgcroots-panel').classList.add('active');
            } else if (panelId === 'heapmergedpaths') {
                document.getElementById('heapmergedpaths-panel').classList.add('active');
            } else if (panelId === 'heaphistogram') {
                document.getElementById('heaphistogram-panel').classList.add('active');
            } else if (panelId === 'heaprootcause') {
                document.getElementById('heaprootcause-panel').classList.add('active');
                // Trigger root cause analysis rendering
                if (summaryData) {
                    HeapAnalysis.renderRootCauseAnalysis(summaryData);
                }
            } else if (panelId === 'heaprefgraph') {
                document.getElementById('heaprefgraph-panel').classList.add('active');
            } else {
                document.getElementById(panelId).classList.add('active');
            }
        },

        searchInFlameGraph(funcName) {
            this.showPanel('flamegraph');
            setTimeout(() => FlameGraph.searchFor(funcName), 100);
        },

        searchInCallGraph(funcName) {
            this.showPanel('callgraph');
            setTimeout(() => CallGraph.searchFor(funcName), 100);
        },

        getCurrentTask() {
            return currentTask;
        },

        getAnalysisType() {
            return currentAnalysisType;
        }
    };
})();

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', function() {
    App.init();
});

// Global function bindings for HTML onclick handlers
window.loadTask = (taskId) => App.loadTask(taskId);
window.showPanel = (panelId) => App.showPanel(panelId);

// Flame Graph bindings
window.searchFlameGraph = () => FlameGraph.search();
window.clearSearch = () => FlameGraph.clearSearch();
window.resetFlameGraph = () => FlameGraph.reset();
window.toggleFlameFilter = (type) => FlameGraph.toggleFilter(type);
window.handleSearchKeyup = (event) => { if (event.key === 'Enter') FlameGraph.search(); };

// Call Graph bindings
window.searchCallGraph = () => CallGraph.search();
window.clearCallGraphSearch = () => CallGraph.clearSearch();
window.fitCallGraph = () => CallGraph.fit();
window.resetCallGraph = () => CallGraph.reset();
window.toggleViewMode = () => CallGraph.toggleViewMode();
window.navigateCallGraphMatch = (dir) => CallGraph.navigateMatch(dir);
window.toggleCallGraphFilter = (type) => CallGraph.toggleFilter(type);
window.toggleCallChainSidebar = (show) => CallGraph.toggleSidebar(show);
window.handleCallGraphSearchKeyup = (event) => {
    if (event.key === 'Enter') CallGraph.search();
    else if (event.key === 'ArrowDown') { event.preventDefault(); CallGraph.navigateMatch(1); }
    else if (event.key === 'ArrowUp') { event.preventDefault(); CallGraph.navigateMatch(-1); }
    else if (event.key === 'Escape') CallGraph.clearSearch();
};

// Heap Analysis bindings
window.filterHeapClasses = () => HeapAnalysis.filterClasses();
window.clearHeapSearch = () => HeapAnalysis.clearSearch();
window.setHeapView = (mode) => HeapAnalysis.setViewMode(mode);
window.togglePackage = (idx) => HeapAnalysis.togglePackage(idx);
window.toggleRetainers = (idx) => HeapAnalysis.toggleRetainers(idx);
window.showGCPathsForClass = (className) => HeapAnalysis.showGCPaths(className);
window.loadRefGraphForClass = (className) => HeapAnalysis.loadRefGraph(className);
window.fitRefGraph = () => HeapAnalysis.fitRefGraph();
window.resetRefGraph = () => HeapAnalysis.resetRefGraph();

// Export for global access
window.App = App;
