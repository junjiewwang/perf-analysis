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
    
    // Remove allocation marker suffix for search
    const cleanName = stripAllocationMarker(funcName);
    
    // For Java-style names like "com.example.Class.method", extract "Class.method" or just "method"
    const parts = cleanName.split('.');
    if (parts.length >= 2) {
        const lastTwo = parts.slice(-2).join('.');
        if (lastTwo.length > 50) {
            return parts[parts.length - 1];
        }
        return lastTwo;
    }
    
    // For C++ style names with ::, extract the last part
    if (cleanName.includes('::')) {
        const cppParts = cleanName.split('::');
        return cppParts[cppParts.length - 1];
    }
    
    return cleanName;
}

// Allocation marker patterns from async-profiler
// _[i] = instance allocation (object count)
// _[k] = size allocation (bytes)
const ALLOCATION_MARKER_REGEX = /_\[(i|k)\]$/;

// Check if function name has allocation marker
function hasAllocationMarker(funcName) {
    if (!funcName) return false;
    return ALLOCATION_MARKER_REGEX.test(funcName);
}

// Get allocation marker type
function getAllocationMarkerType(funcName) {
    if (!funcName) return null;
    const match = funcName.match(ALLOCATION_MARKER_REGEX);
    if (!match) return null;
    return match[1] === 'i' ? 'instance' : 'size';
}

// Strip allocation marker from function name
function stripAllocationMarker(funcName) {
    if (!funcName) return '';
    return funcName.replace(ALLOCATION_MARKER_REGEX, '');
}

// Format function name for display with allocation info
// Returns { displayName, badge, tooltip, originalName }
function formatFunctionName(funcName, options = {}) {
    if (!funcName) return { displayName: '', badge: '', tooltip: '', originalName: '' };
    
    const { showBadge = true, showTooltip = true } = options;
    const markerType = getAllocationMarkerType(funcName);
    const cleanName = stripAllocationMarker(funcName);
    
    let badge = '';
    let tooltip = '';
    
    if (markerType && showBadge) {
        if (markerType === 'instance') {
            badge = '<span class="alloc-badge alloc-instance" title="Instance allocation (object count)">📦 inst</span>';
            tooltip = 'Instance allocation - counts number of objects allocated';
        } else {
            badge = '<span class="alloc-badge alloc-size" title="Size allocation (bytes)">📊 size</span>';
            tooltip = 'Size allocation - measures bytes allocated';
        }
    }
    
    return {
        displayName: cleanName,
        badge: badge,
        tooltip: showTooltip ? tooltip : '',
        originalName: funcName,
        isAllocation: !!markerType,
        allocationType: markerType
    };
}

// Format function name as HTML with badge
function formatFunctionNameHtml(funcName, options = {}) {
    const formatted = formatFunctionName(funcName, options);
    if (!formatted.badge) {
        return Utils.escapeHtml(formatted.displayName);
    }
    return `${Utils.escapeHtml(formatted.displayName)} ${formatted.badge}`;
}

/**
 * Parse method name to extract package, class, and method components.
 * Supports Java, Go, C++, Native library, and simple function name formats.
 * 
 * @param {string} fullName - Full qualified function name
 * @returns {Object} Parsed result with type, package, class, method, and display info
 */
function parseMethodName(fullName) {
    if (!fullName) {
        return { type: 'simple', package: '', class: '', method: '', fullName: '' };
    }

    // Strip allocation marker first
    const cleanName = stripAllocationMarker(fullName);

    // Pattern 1: Native library format - /path/to/lib.so::function or lib.so::function
    const nativeLibMatch = cleanName.match(/^(.+\.so(?:\.[0-9.]+)?)::([\w_]+)$/);
    if (nativeLibMatch) {
        const libPath = nativeLibMatch[1];
        const funcName = nativeLibMatch[2];
        // Extract just the library name from path
        const libName = libPath.split('/').pop();
        return {
            type: 'native_lib',
            package: libPath.includes('/') ? libPath.substring(0, libPath.lastIndexOf('/')) : '',
            class: libName,
            method: funcName,
            fullName: cleanName
        };
    }

    // Pattern 2: Kernel or special tags - [kernel], [native], etc.
    const tagMatch = cleanName.match(/^\[(\w+)\]\s*(.*)$/);
    if (tagMatch) {
        return {
            type: 'kernel',
            package: '',
            class: `[${tagMatch[1]}]`,
            method: tagMatch[2] || '',
            fullName: cleanName
        };
    }

    // Pattern 3: C++ style - Namespace::Class::method or std::vector<T>::push_back
    // Also handles mangled names like _ZN...
    if (cleanName.includes('::') && !cleanName.includes('.')) {
        const parts = cleanName.split('::');
        const method = parts.pop() || '';
        const className = parts.pop() || '';
        const namespace = parts.join('::');
        return {
            type: 'cpp',
            package: namespace,
            class: className,
            method: method,
            fullName: cleanName
        };
    }

    // Pattern 4: Go style - github.com/pkg/errors.Wrap or runtime.goexit
    // Go packages use / for path and . for final separator
    const goMatch = cleanName.match(/^((?:[\w-]+\/)*[\w-]+(?:\/[\w-]+)*)\.(\w+)$/);
    if (goMatch && cleanName.includes('/')) {
        const pkgPath = goMatch[1];
        const funcName = goMatch[2];
        return {
            type: 'go',
            package: pkgPath,
            class: pkgPath.split('/').pop() || '',
            method: funcName,
            fullName: cleanName
        };
    }

    // Pattern 5: Java style - com.example.package.Class.method or Class.method
    // Java uses dots throughout
    if (cleanName.includes('.') && !cleanName.includes('/') && !cleanName.includes('::')) {
        const parts = cleanName.split('.');
        if (parts.length >= 2) {
            const method = parts.pop() || '';
            const className = parts.pop() || '';
            const packagePath = parts.join('.');
            return {
                type: 'java',
                package: packagePath,
                class: className,
                method: method,
                fullName: cleanName
            };
        }
    }

    // Pattern 6: Pure file path - /usr/lib/something
    if (cleanName.startsWith('/') && !cleanName.includes('::')) {
        const pathParts = cleanName.split('/');
        const fileName = pathParts.pop() || '';
        return {
            type: 'path',
            package: pathParts.join('/'),
            class: '',
            method: fileName,
            fullName: cleanName
        };
    }

    // Pattern 7: Simple function name
    return {
        type: 'simple',
        package: '',
        class: '',
        method: cleanName,
        fullName: cleanName
    };
}

/**
 * Format parsed method name for display with visual hierarchy.
 * Returns HTML string with styled package, class, and method.
 * 
 * @param {Object} parsed - Result from parseMethodName()
 * @param {Object} options - Display options
 * @returns {string} Formatted HTML string
 */
function formatParsedMethodName(parsed, options = {}) {
    const { maxPackageLen = 60, maxClassLen = 40, maxMethodLen = 50 } = options;
    
    let html = '';
    
    // Package/namespace line (if exists)
    if (parsed.package) {
        let pkgDisplay = parsed.package;
        if (pkgDisplay.length > maxPackageLen) {
            pkgDisplay = '...' + pkgDisplay.slice(-maxPackageLen + 3);
        }
        const icon = parsed.type === 'native_lib' ? '📁' : 
                     parsed.type === 'kernel' ? '🔧' :
                     parsed.type === 'go' ? '🐹' :
                     parsed.type === 'cpp' ? '⚙️' : '📦';
        html += `<div class="tooltip-package">${icon} ${escapeHtml(pkgDisplay)}</div>`;
    }
    
    // Class line (if exists)
    if (parsed.class) {
        let classDisplay = parsed.class;
        if (classDisplay.length > maxClassLen) {
            classDisplay = classDisplay.slice(0, maxClassLen - 2) + '..';
        }
        html += `<div class="tooltip-class">🔷 ${escapeHtml(classDisplay)}</div>`;
    }
    
    // Method line
    if (parsed.method) {
        let methodDisplay = parsed.method;
        if (methodDisplay.length > maxMethodLen) {
            methodDisplay = methodDisplay.slice(0, maxMethodLen - 2) + '..';
        }
        const arrow = (parsed.class || parsed.package) ? '→ ' : '';
        html += `<div class="tooltip-method">${arrow}${escapeHtml(methodDisplay)}</div>`;
    }
    
    return html;
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
    // Allocation marker utilities
    hasAllocationMarker,
    getAllocationMarkerType,
    stripAllocationMarker,
    formatFunctionName,
    formatFunctionNameHtml,
    // Method name parsing utilities
    parseMethodName,
    formatParsedMethodName,
    SYSTEM_PATTERNS
};

window.Utils = Utils;
