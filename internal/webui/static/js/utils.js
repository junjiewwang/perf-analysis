/**
 * Utility functions - Common helper functions used across modules
 */

// Format bytes to human readable
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Format number with commas
function formatNumber(num) {
    return num.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

// Format datetime string
function formatDateTime(isoString) {
    try {
        const date = new Date(isoString);
        return date.toLocaleString();
    } catch (e) {
        return isoString;
    }
}

// Format duration in milliseconds
function formatDuration(ms) {
    if (ms < 1000) {
        return `${ms}ms`;
    } else if (ms < 60000) {
        return `${(ms / 1000).toFixed(2)}s`;
    } else {
        const minutes = Math.floor(ms / 60000);
        const seconds = ((ms % 60000) / 1000).toFixed(1);
        return `${minutes}m ${seconds}s`;
    }
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Truncate text for display
function truncateText(text, maxLen) {
    if (!text) return '';
    const parts = text.split('.');
    const shortName = parts[parts.length - 1] || text;
    if (shortName.length <= maxLen) return shortName;
    return shortName.substring(0, maxLen - 2) + '..';
}

// Get short class name from full qualified name
function getShortClassName(fullName) {
    if (!fullName) return 'unknown';
    const parts = fullName.split('.');
    return parts[parts.length - 1];
}

// Extract a meaningful search term from full function name
function extractSearchTerm(funcName) {
    if (!funcName) return '';
    
    // For Java-style names like "com.example.Class.method", extract "Class.method" or just "method"
    const parts = funcName.split('.');
    if (parts.length >= 2) {
        const lastTwo = parts.slice(-2).join('.');
        if (lastTwo.length > 50) {
            return parts[parts.length - 1];
        }
        return lastTwo;
    }
    
    // For C++ style names with ::, extract the last part
    if (funcName.includes('::')) {
        const cppParts = funcName.split('::');
        return cppParts[cppParts.length - 1];
    }
    
    return funcName;
}

// Copy text to clipboard
function copyToClipboard(text, element) {
    navigator.clipboard.writeText(text).then(() => {
        if (element) {
            const originalTitle = element.title;
            element.title = '✓ Copied!';
            setTimeout(() => {
                element.title = originalTitle;
            }, 1500);
        }
    }).catch(err => {
        console.error('Failed to copy:', err);
    });
}

// Deep clone an object
function deepClone(obj) {
    if (obj === null || typeof obj !== 'object') return obj;
    if (Array.isArray(obj)) {
        return obj.map(item => deepClone(item));
    }
    const cloned = {};
    for (const key in obj) {
        if (obj.hasOwnProperty(key)) {
            cloned[key] = deepClone(obj[key]);
        }
    }
    return cloned;
}

// System function patterns for filtering
const SYSTEM_PATTERNS = {
    jvm: [
        /^java\./,
        /^javax\./,
        /^jdk\./,
        /^sun\./,
        /^com\.sun\./,
        /^org\.openjdk\./,
        /^java\.lang\./,
        /^java\.util\./,
        /^java\.io\./,
        /^java\.nio\./,
        /^java\.net\./,
        /^java\.security\./,
        /^java\.concurrent\./,
        /Unsafe\./,
        /^jdk\.internal\./
    ],
    gc: [
        /GC/i,
        /Garbage/i,
        /^G1/,
        /^ZGC/,
        /^Shenandoah/,
        /ParallelGC/,
        /ConcurrentMark/,
        /SafePoint/i,
        /safepoint/i,
        /VMThread/,
        /Reference.*Handler/
    ],
    native: [
        /^\[native\]/,
        /^libc\./,
        /^libpthread/,
        /^ld-linux/,
        /^__/,
        /^_Z/,
        /^std::/,
        /::operator/,
        /^pthread_/,
        /^malloc/,
        /^free$/,
        /^mmap/,
        /^munmap/,
        /^brk$/,
        /^clone$/,
        /^futex/,
        /^epoll_/,
        /^syscall/
    ],
    kernel: [
        /^\[kernel\]/,
        /^vmlinux/,
        /^do_syscall/,
        /^sys_/,
        /^__x64_sys/,
        /^entry_SYSCALL/,
        /^page_fault/,
        /^handle_mm_fault/,
        /^__schedule/,
        /^schedule$/,
        /^ret_from_fork/,
        /^irq_/,
        /^softirq/,
        /^ksoftirqd/,
        /^kworker/,
        /^rcu_/
    ],
    reflect: [
        /reflect/i,
        /Reflect/,
        /\$Proxy/,
        /CGLIB/,
        /ByteBuddy/,
        /javassist/,
        /MethodHandle/,
        /LambdaForm/,
        /GeneratedMethodAccessor/,
        /DelegatingMethodAccessor/,
        /NativeMethodAccessor/,
        /invoke0/,
        /invokespecial/,
        /invokevirtual/
    ]
};

// Check if a function name matches system patterns
function isSystemFunction(name, filterTypes) {
    if (!name || filterTypes.size === 0) return false;
    
    for (const filterType of filterTypes) {
        const patterns = SYSTEM_PATTERNS[filterType];
        if (patterns) {
            for (const pattern of patterns) {
                if (pattern.test(name)) {
                    return true;
                }
            }
        }
    }
    return false;
}

// Get CSS class for task type badge
function getTaskTypeClass(typeName) {
    const typeMap = {
        'java': 'java',
        'generic': 'generic',
        'pprof_mem': 'pprof',
        'memleak': 'memory',
        'java_heap': 'heap',
        'phys_mem': 'memory',
        'jeprof': 'memory',
        'tracing': 'tracing',
        'timing': 'tracing'
    };
    return typeMap[typeName] || 'generic';
}

// Get icon for task type
function getTaskTypeIcon(typeName) {
    const iconMap = {
        'java': '☕',
        'generic': '🔧',
        'pprof_mem': '🐹',
        'memleak': '💾',
        'java_heap': '📦',
        'phys_mem': '🧠',
        'jeprof': '📊',
        'tracing': '🔍',
        'timing': '⏱️',
        'bolt': '⚡'
    };
    return iconMap[typeName] || '📊';
}

// Get icon for profiler
function getProfilerIcon(profilerName) {
    const iconMap = {
        'perf': '🔥',
        'async_alloc': '📈',
        'pprof': '🐹'
    };
    return iconMap[profilerName] || '📊';
}

// Export for use in other modules
const Utils = {
    formatBytes,
    formatNumber,
    formatDateTime,
    formatDuration,
    escapeHtml,
    truncateText,
    getShortClassName,
    extractSearchTerm,
    copyToClipboard,
    deepClone,
    isSystemFunction,
    getTaskTypeClass,
    getTaskTypeIcon,
    getProfilerIcon,
    SYSTEM_PATTERNS
};

window.Utils = Utils;
