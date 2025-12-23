/**
 * Main Application Module
 * Legacy bindings for backward compatibility with existing onclick handlers
 * Primary state management now handled by Alpine.js appState()
 */

const App = (function() {
    // Helper to get Alpine.js app data
    function getAlpineAppData() {
        const appEl = document.querySelector('[x-data]');
        if (appEl && appEl._x_dataStack && appEl._x_dataStack.length > 0) {
            return appEl._x_dataStack[0];
        }
        return null;
    }

    // These methods are kept for backward compatibility with existing code
    return {
        // Legacy init - now handled by Alpine.js
        async init() {
            console.log('App.init() called - initialization handled by Alpine.js');
        },

        // Search in flame graph (legacy binding)
        searchInFlameGraph(funcName) {
            const appData = getAlpineAppData();
            if (appData && typeof appData.searchInFlameGraph === 'function') {
                appData.searchInFlameGraph(funcName);
            } else {
                // Fallback for direct calls when Alpine not ready
                window.showPanel && window.showPanel('flamegraph');
                setTimeout(() => FlameGraph.searchFor(funcName), 100);
            }
        },

        // Search in call graph (legacy binding)
        searchInCallGraph(funcName) {
            const appData = getAlpineAppData();
            if (appData && typeof appData.searchInCallGraph === 'function') {
                appData.searchInCallGraph(funcName);
            } else {
                // Fallback for direct calls when Alpine not ready
                window.showPanel && window.showPanel('callgraph');
                setTimeout(() => CallGraph.searchFor(funcName), 100);
            }
        },

        // Get current task (legacy)
        getCurrentTask() {
            const appData = getAlpineAppData();
            return appData ? appData.currentTask : '';
        },

        // Get analysis type (legacy)
        getAnalysisType() {
            const appData = getAlpineAppData();
            return appData ? appData.analysisType : 'cpu';
        },

        // Get summary data (for other modules)
        getSummaryData() {
            const appData = getAlpineAppData();
            return appData ? appData.summaryData : null;
        },

        // Show panel (legacy)
        showPanel(panelId) {
            const appData = getAlpineAppData();
            if (appData && typeof appData.showPanel === 'function') {
                appData.showPanel(panelId);
            }
        },

        // Load task (legacy)
        loadTask(taskId) {
            const appData = getAlpineAppData();
            if (appData && typeof appData.loadTask === 'function') {
                appData.loadTask(taskId);
            }
        }
    };
})();

// Global function bindings for HTML onclick handlers (legacy support)
window.loadTask = (taskId) => App.loadTask(taskId);
window.showPanel = (panelId) => App.showPanel(panelId);

// Flame Graph bindings
window.searchFlameGraph = () => FlameGraph.search();
window.clearSearch = () => FlameGraph.clearSearch();
window.resetFlameGraph = () => FlameGraph.reset();
window.toggleFlameFilter = (type) => FlameGraph.toggleFilter(type);
window.handleSearchKeyup = (event) => { if (event.key === 'Enter') FlameGraph.search(); };
// Flame Graph Thread Selector bindings
window.selectFlameThread = (tid) => FlameGraph.selectThread(tid);
window.toggleFlameThreadDropdown = (show) => FlameGraph.toggleThreadDropdown(show);
window.handleFlameThreadSearch = (value) => FlameGraph.handleThreadSearch(value);
window.handleFlameThreadSearchKeydown = (event) => FlameGraph.handleThreadSearchKeydown(event);

// Call Graph bindings
window.searchCallGraph = () => CallGraph.search();
window.clearCallGraphSearch = () => CallGraph.clearSearch();
window.fitCallGraph = () => CallGraph.fit();
window.resetCallGraph = () => CallGraph.reset();
window.toggleViewMode = () => CallGraph.toggleViewMode();
window.navigateCallGraphMatch = (dir) => CallGraph.navigateMatch(dir);
window.toggleCallGraphFilter = (type) => CallGraph.toggleFilter(type);
window.toggleCallChainSidebar = (show) => CallGraph.toggleSidebar(show);
window.selectCallGraphThread = (tid) => CallGraph.selectThread(tid);
window.toggleThreadDropdown = (show) => CallGraph.toggleThreadDropdown(show);
window.handleThreadSearch = (value) => CallGraph.handleThreadSearch(value);
window.handleThreadSearchKeydown = (event) => CallGraph.handleThreadSearchKeydown(event);
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
