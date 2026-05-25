/**
 * API Module - Handles all API calls to the backend
 */

const API = {
    // Fetch list of available tasks
    async getTasks() {
        const response = await fetch('/api/tasks');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch summary data for a task
    async getSummary(taskId) {
        const response = await fetch(`/api/summary?task=${taskId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch flame graph data for a task
    // type: 'cpu' (default), 'memory', 'alloc', 'tracing'
    async getFlameGraph(taskId, type = '') {
        let url = `/api/flamegraph?task=${taskId}`;
        if (type) {
            url += `&type=${type}`;
        }
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch call graph data for a task
    // type: 'cpu' (default), 'memory', 'alloc'
    async getCallGraph(taskId, type = '') {
        let url = `/api/callgraph?task=${taskId}`;
        if (type) {
            url += `&type=${type}`;
        }
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch heap analysis data (histogram)
    async getHeapHistogram(taskId) {
        const response = await fetch(`/api/heap/histogram?task=${taskId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch GC roots summary (via HeapQueryEngine on-demand computation)
    async getGCRootsSummary(taskId) {
        const response = await fetch(`/api/refgraph/gc-roots-summary?task=${taskId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch GC roots list
    async getGCRootsList(taskId) {
        const response = await fetch(`/api/refgraph/gc-roots-list?task=${taskId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch objects retained by a specific GC root
    async getGCRootRetained(taskId, objectId, maxObjects = 50) {
        const response = await fetch(`/api/refgraph/gc-root-retained?task=${taskId}&id=${objectId}&max=${maxObjects}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch object fields using refgraph
    async getObjectFields(taskId, objectId) {
        const response = await fetch(`/api/refgraph/fields?task=${taskId}&id=${objectId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch dominator tree children for a given object (or virtual root if no id)
    async getDominatorChildren(taskId, objectId = '', topN = 50, sortBy = 'retained') {
        let url = `/api/refgraph/dominator-tree?task=${taskId}&top=${topN}&sort=${sortBy}`;
        if (objectId) {
            url += `&id=${encodeURIComponent(objectId)}`;
        }
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch dominator path from root to a specific object
    async getDominatorPath(taskId, objectId) {
        const response = await fetch(`/api/refgraph/dominator-path?task=${taskId}&id=${encodeURIComponent(objectId)}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch retained size treemap data
    async getRetainedSizeTreemap(taskId, rootId = '', maxNodes = 200) {
        let url = `/api/refgraph/treemap?task=${taskId}&maxNodes=${maxNodes}`;
        if (rootId) {
            url += `&root=${encodeURIComponent(rootId)}`;
        }
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch class-level retainers (who holds references to instances of this class)
    async getClassRetainers(taskId, className, topN = 20) {
        const response = await fetch(`/api/refgraph/class-retainers?task=${taskId}&class=${encodeURIComponent(className)}&top=${topN}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    }
};

// Export for use in other modules
window.API = API;
