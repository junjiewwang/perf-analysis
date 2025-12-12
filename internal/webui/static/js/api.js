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
    async getFlameGraph(taskId) {
        const response = await fetch(`/api/flamegraph?task=${taskId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        return response.json();
    },

    // Fetch call graph data for a task
    async getCallGraph(taskId) {
        const response = await fetch(`/api/callgraph?task=${taskId}`);
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
    }
};

// Export for use in other modules
window.API = API;
