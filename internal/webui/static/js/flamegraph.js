/**
 * Flame Graph Module
 * Handles flame graph rendering, search, and filtering
 */

const FlameGraph = (function() {
    // Private state
    let flameChart = null;
    let flameGraphData = null;
    let originalFlameGraphData = null;
    let currentSearchTerm = '';
    let flameFilters = new Set();
    let isInverted = true;

    // Thread selector state
    let threadFlameGraphs = [];      // Threads with flame_root data
    let selectedThreadTid = null;    // Current selected thread TID, null = global view
    let hasThreadData = false;       // Whether thread flame graph data is available
    let originalApiData = null;      // Original API response data

    // System function patterns for filtering
    const SYSTEM_PATTERNS = {
        jvm: [
            /^java\./, /^javax\./, /^jdk\./, /^sun\./, /^com\.sun\./,
            /^org\.openjdk\./, /^java\.lang\./, /^java\.util\./,
            /^java\.io\./, /^java\.nio\./, /^java\.net\./,
            /^java\.security\./, /^java\.concurrent\./, /Unsafe\./,
            /^jdk\.internal\./
        ],
        gc: [
            /GC/i, /Garbage/i, /^G1/, /^ZGC/, /^Shenandoah/,
            /ParallelGC/, /ConcurrentMark/, /SafePoint/i, /safepoint/i,
            /VMThread/, /Reference.*Handler/
        ],
        native: [
            /^\[native\]/, /^libc\./, /^libpthread/, /^ld-linux/,
            /^__/, /^_Z/, /^std::/, /::operator/, /^pthread_/,
            /^malloc/, /^free$/, /^mmap/, /^munmap/, /^brk$/,
            /^clone$/, /^futex/, /^epoll_/, /^syscall/
        ],
        kernel: [
            /^\[kernel\]/, /^vmlinux/, /^do_syscall/, /^sys_/,
            /^__x64_sys/, /^entry_SYSCALL/, /^page_fault/,
            /^handle_mm_fault/, /^__schedule/, /^schedule$/,
            /^ret_from_fork/, /^irq_/, /^softirq/, /^ksoftirqd/,
            /^kworker/, /^rcu_/
        ],
        reflect: [
            /reflect/i, /Reflect/, /\$Proxy/, /CGLIB/, /ByteBuddy/,
            /javassist/, /MethodHandle/, /LambdaForm/,
            /GeneratedMethodAccessor/, /DelegatingMethodAccessor/,
            /NativeMethodAccessor/, /invoke0/, /invokespecial/,
            /invokevirtual/
        ]
    };

    // Custom tooltip using Tippy.js with mouse-following behavior
    // d3-flamegraph requires tooltip to be a function (for svg.call) with show/hide methods
    let flameTooltipInstance = null;
    let tippyInstance = null;
    
    // Mouse position tracking for tooltip follow
    let mouseX = 0;
    let mouseY = 0;
    let isTooltipVisible = false;
    let mouseMoveHandler = null;
    let currentTooltipData = null;
    
    function createFlameTooltip() {
        // Return existing instance if already created
        if (flameTooltipInstance) {
            return flameTooltipInstance;
        }
        
        // Create a virtual element for Tippy to attach to (follows mouse)
        const virtualElement = {
            getBoundingClientRect: () => ({
                width: 0,
                height: 0,
                top: mouseY,
                left: mouseX,
                right: mouseX,
                bottom: mouseY
            })
        };
        
        // Build tooltip HTML content
        function buildTooltipContent(d) {
            const name = d.data.name || '';
            const value = d.value || 0;
            const self = d.data.self || 0;
            const module = d.data.module || '';
            
            // Parse function name for structured display
            const parsed = Utils.parseMethodName(name);
            const formatted = Utils.formatFunctionName(name, { showBadge: false });
            
            // Calculate percentages
            let totalPct = 0, selfPct = 0;
            if (flameGraphData && flameGraphData.value > 0) {
                totalPct = (value / flameGraphData.value * 100);
                selfPct = (self / flameGraphData.value * 100);
            }
            
            // Build tooltip HTML
            let html = '<div class="flame-tippy-content">';
            
            // Function name section
            html += '<div class="tippy-func-section">';
            if (parsed.method) {
                html += `<div class="tippy-func-name">${Utils.escapeHtml(parsed.method)}</div>`;
            } else {
                html += `<div class="tippy-func-name">${Utils.escapeHtml(name)}</div>`;
            }
            if (parsed.class) {
                html += `<div class="tippy-class-name">${Utils.escapeHtml(parsed.class)}</div>`;
            }
            if (parsed.package) {
                let pkgDisplay = parsed.package;
                if (pkgDisplay.length > 50) {
                    pkgDisplay = '...' + pkgDisplay.slice(-47);
                }
                html += `<div class="tippy-package">${Utils.escapeHtml(pkgDisplay)}</div>`;
            }
            html += '</div>';
            
            // Allocation badge
            if (formatted.isAllocation) {
                const allocLabel = formatted.allocationType === 'instance' ? 'Instance' : 'Size';
                const allocClass = formatted.allocationType === 'instance' ? 'instance' : 'size';
                html += `<div class="tippy-alloc-badge tippy-alloc-${allocClass}">${allocLabel} Allocation</div>`;
            }
            
            // Stats section
            html += '<div class="tippy-stats">';
            html += `<div class="tippy-stat-row">`;
            html += `<span class="tippy-stat-label">Total</span>`;
            html += `<span class="tippy-stat-value">${value.toLocaleString()}</span>`;
            html += `<span class="tippy-stat-pct">${totalPct.toFixed(2)}%</span>`;
            html += `</div>`;
            
            if (self > 0) {
                html += `<div class="tippy-stat-row">`;
                html += `<span class="tippy-stat-label">Self</span>`;
                html += `<span class="tippy-stat-value">${self.toLocaleString()}</span>`;
                html += `<span class="tippy-stat-pct">${selfPct.toFixed(2)}%</span>`;
                html += `</div>`;
                // Progress bar
                const barWidth = Math.min(100, Math.max(0, selfPct));
                html += `<div class="tippy-progress"><div class="tippy-progress-fill" style="width:${barWidth}%"></div></div>`;
            }
            html += '</div>';
            
            // Module
            if (module) {
                html += `<div class="tippy-module">${Utils.escapeHtml(module)}</div>`;
            }
            
            // Hint
            html += '<div class="tippy-hint">Click to zoom · Right-click to zoom out</div>';
            
            html += '</div>';
            return html;
        }
        
        // Update tooltip position based on current mouse coordinates
        function updateTooltipPosition() {
            if (tippyInstance && isTooltipVisible) {
                tippyInstance.setProps({
                    getReferenceClientRect: () => ({
                        width: 0,
                        height: 0,
                        top: mouseY,
                        left: mouseX,
                        right: mouseX,
                        bottom: mouseY
                    })
                });
            }
        }
        
        // Throttled mouse move handler using requestAnimationFrame
        let rafId = null;
        function handleMouseMove(e) {
            mouseX = e.clientX;
            mouseY = e.clientY;
            
            // Use RAF for smooth updates without overwhelming the browser
            if (!rafId) {
                rafId = requestAnimationFrame(() => {
                    updateTooltipPosition();
                    rafId = null;
                });
            }
        }
        
        // Create the tooltip function that d3 will call
        function tooltip(selection) {
            // Initialize Tippy instance if not exists
            if (!tippyInstance && typeof tippy !== 'undefined') {
                tippyInstance = tippy(document.body, {
                    getReferenceClientRect: () => virtualElement.getBoundingClientRect(),
                    appendTo: document.body,
                    content: '',
                    allowHTML: true,
                    placement: 'right-start',
                    trigger: 'manual',
                    interactive: false,
                    arrow: true,
                    theme: 'flame',
                    maxWidth: 400,
                    offset: [10, 15], // Offset from mouse cursor
                    animation: 'shift-away',
                    duration: [100, 75], // Faster show/hide for mouse following
                    hideOnClick: false,
                    popperOptions: {
                        modifiers: [
                            {
                                name: 'flip',
                                options: {
                                    fallbackPlacements: ['left-start', 'top', 'bottom']
                                }
                            },
                            {
                                name: 'preventOverflow',
                                options: {
                                    boundary: 'viewport',
                                    padding: 10
                                }
                            }
                        ]
                    }
                });
            }
            
            // Setup mouse move listener on the flame graph container
            const container = document.getElementById('flamegraph');
            if (container && !mouseMoveHandler) {
                mouseMoveHandler = handleMouseMove;
                container.addEventListener('mousemove', mouseMoveHandler, { passive: true });
            }
        }
        
        // Attach show method
        tooltip.show = function(d, element) {
            if (!tippyInstance) {
                tooltip(); // Initialize if needed
            }
            
            if (tippyInstance) {
                currentTooltipData = d;
                isTooltipVisible = true;
                
                // Set content and show at current mouse position
                tippyInstance.setContent(buildTooltipContent(d));
                tippyInstance.setProps({
                    getReferenceClientRect: () => ({
                        width: 0,
                        height: 0,
                        top: mouseY,
                        left: mouseX,
                        right: mouseX,
                        bottom: mouseY
                    })
                });
                tippyInstance.show();
            }
        };
        
        // Attach hide method
        tooltip.hide = function() {
            isTooltipVisible = false;
            currentTooltipData = null;
            if (tippyInstance) {
                tippyInstance.hide();
            }
        };
        
        // Attach destroy method
        tooltip.destroy = function() {
            isTooltipVisible = false;
            currentTooltipData = null;
            
            // Remove mouse move listener
            const container = document.getElementById('flamegraph');
            if (container && mouseMoveHandler) {
                container.removeEventListener('mousemove', mouseMoveHandler);
                mouseMoveHandler = null;
            }
            
            if (tippyInstance) {
                tippyInstance.destroy();
                tippyInstance = null;
            }
        };
        
        flameTooltipInstance = tooltip;
        return tooltip;
    }

    // Transform flame graph data to d3-flamegraph format
    // Preserves self, module, and other metadata for tooltip display
    function transformFlameData(data) {
        if (!data) return null;
        const node = data.root || data;
        const result = {
            name: node.func || node.name || 'root',
            value: node.value || 0,
            self: node.self || 0,
            module: node.module || '',
            children: []
        };
        if (node.children && Array.isArray(node.children)) {
            result.children = node.children.map(child => transformFlameData(child)).filter(c => c !== null);
        }
        return result;
    }

    // Deep clone flame graph data
    function deepCloneFlameData(node) {
        if (!node) return null;
        return {
            name: node.name,
            value: node.value,
            self: node.self || 0,
            module: node.module || '',
            children: node.children ? node.children.map(c => deepCloneFlameData(c)) : []
        };
    }

    // Calculate flame graph statistics
    function calculateFlameStats(node, depth = 0, funcSet = new Set()) {
        if (!node) return { totalSamples: 0, uniqueFuncs: 0, maxDepth: 0 };
        funcSet.add(node.name);
        let maxDepth = depth;
        if (node.children && node.children.length > 0) {
            for (const child of node.children) {
                const childStats = calculateFlameStats(child, depth + 1, funcSet);
                maxDepth = Math.max(maxDepth, childStats.maxDepth);
            }
        }
        return {
            totalSamples: node.value || 0,
            uniqueFuncs: funcSet.size,
            maxDepth: maxDepth
        };
    }

    // Check if a function name matches system patterns
    function isSystemFunction(name, filterTypes) {
        if (!name || filterTypes.size === 0) return false;
        for (const filterType of filterTypes) {
            const patterns = SYSTEM_PATTERNS[filterType];
            if (patterns) {
                for (const pattern of patterns) {
                    if (pattern.test(name)) return true;
                }
            }
        }
        return false;
    }

    // Collapse filtered nodes
    function collapseFilteredNodes(node, filterTypes) {
        if (!node) return null;
        const cloned = deepCloneFlameData(node);
        const result = {
            name: cloned.name,
            value: cloned.value,
            children: []
        };
        if (cloned.children && cloned.children.length > 0) {
            result.children = processChildrenForCollapse(cloned.children, filterTypes);
        }
        recalculateValues(result);
        return result;
    }

    function processChildrenForCollapse(children, filterTypes) {
        const resultChildren = [];
        for (const child of children) {
            const isFiltered = isSystemFunction(child.name, filterTypes);
            if (isFiltered) {
                if (child.children && child.children.length > 0) {
                    const promotedChildren = processChildrenForCollapse(child.children, filterTypes);
                    resultChildren.push(...promotedChildren);
                }
            } else {
                const processedChild = {
                    name: child.name,
                    value: child.value || 0,
                    children: []
                };
                if (child.children && child.children.length > 0) {
                    processedChild.children = processChildrenForCollapse(child.children, filterTypes);
                }
                resultChildren.push(processedChild);
            }
        }
        return mergeFlameChildren(resultChildren);
    }

    function mergeFlameChildren(children) {
        if (!children || children.length === 0) return [];
        const nameMap = new Map();
        for (const child of children) {
            if (nameMap.has(child.name)) {
                const existing = nameMap.get(child.name);
                existing.value = (existing.value || 0) + (child.value || 0);
                if (child.children && child.children.length > 0) {
                    existing.children = existing.children || [];
                    for (const grandchild of child.children) {
                        existing.children.push(deepCloneFlameData(grandchild));
                    }
                }
            } else {
                nameMap.set(child.name, {
                    name: child.name,
                    value: child.value || 0,
                    children: child.children ? child.children.map(c => deepCloneFlameData(c)) : []
                });
            }
        }
        const result = [];
        for (const [name, node] of nameMap) {
            if (node.children.length > 0) {
                node.children = mergeFlameChildren(node.children);
            }
            result.push(node);
        }
        return result;
    }

    function recalculateValues(node) {
        if (!node.children || node.children.length === 0) {
            return node.value || 0;
        }
        let childSum = 0;
        for (const child of node.children) {
            childSum += recalculateValues(child);
        }
        node.value = Math.max(node.value || 0, childSum);
        return node.value;
    }

    function validateFlameData(node) {
        if (!node) return 0;
        if (typeof node.value !== 'number' || isNaN(node.value)) {
            node.value = 0;
        }
        if (!Array.isArray(node.children)) {
            node.children = [];
        }
        let childSum = 0;
        for (const child of node.children) {
            childSum += validateFlameData(child);
        }
        if (node.children.length > 0) {
            node.value = Math.max(node.value, childSum);
        }
        return node.value;
    }

    // Apply search highlight
    function applySearchHighlight(term) {
        if (!term) return 0;
        const termLower = term.toLowerCase();
        // Also search without allocation marker for better UX
        const termClean = Utils.stripAllocationMarker(term).toLowerCase();
        let matchCount = 0;
        
        d3.select('#flamegraph').selectAll('.d3-flame-graph g').each(function() {
            const g = d3.select(this);
            
            // Try multiple ways to get the function name:
            // 1. From __data__ (d3 bound data) - most reliable
            // 2. From title element
            // 3. From text element
            let name = '';
            
            // Method 1: Get from d3 bound data (most reliable for d3-flamegraph)
            const data = g.datum();
            if (data && data.data && data.data.name) {
                name = data.data.name;
            } else if (data && data.name) {
                name = data.name;
            }
            
            // Method 2: Fallback to title element
            if (!name) {
                const titleEl = g.select('title');
                if (!titleEl.empty()) {
                    name = titleEl.text();
                }
            }
            
            // Method 3: Fallback to text element
            if (!name) {
                const textEl = g.select('text');
                if (!textEl.empty()) {
                    name = textEl.text();
                }
            }
            
            if (!name) {
                g.classed('search-match', false);
                return;
            }
            
            const nameLower = name.toLowerCase();
            // Match original name or name without allocation marker
            const nameClean = Utils.stripAllocationMarker(name).toLowerCase();
            if (nameLower.includes(termLower) || nameClean.includes(termClean)) {
                g.classed('search-match', true);
                matchCount++;
            } else {
                g.classed('search-match', false);
            }
        });
        return matchCount;
    }

    // Public API
    return {
        init: function() {
            // Initialize event listeners
            const searchInput = document.getElementById('searchInput');
            if (searchInput) {
                searchInput.addEventListener('keyup', (e) => {
                    if (e.key === 'Enter') this.search();
                });
            }
        },

        async load(taskId, type = '') {
            const container = document.getElementById('flamegraph');
            container.innerHTML = '<div class="loading">Loading flame graph</div>';

            try {
                const data = await API.getFlameGraph(taskId, type);
                originalApiData = data;
                flameGraphData = transformFlameData(data);
                originalFlameGraphData = deepCloneFlameData(flameGraphData);

                // Extract thread flame graphs if available
                threadFlameGraphs = [];
                hasThreadData = false;
                selectedThreadTid = null;

                if (data.thread_analysis && data.thread_analysis.threads) {
                    // Filter threads that have flame_root data
                    threadFlameGraphs = data.thread_analysis.threads
                        .filter(t => t.flame_root && t.flame_root.children && t.flame_root.children.length > 0)
                        .sort((a, b) => (b.samples || 0) - (a.samples || 0));
                    hasThreadData = threadFlameGraphs.length > 0;
                }

                // Update thread selector UI
                this.updateThreadSelector();

                if (!flameGraphData || flameGraphData.value === 0) {
                    container.innerHTML = '<div class="loading">No flame graph data available</div>';
                    return;
                }

                this.render();
            } catch (err) {
                console.error('Failed to load flame graph:', err);
                container.innerHTML = '<div class="loading">Failed to load flame graph: ' + err.message + '</div>';
            }
        },

        render() {
            const container = document.getElementById('flamegraph');

            container.innerHTML = '';

            // 检查容器是否可见（兼容 Alpine.js x-show 和传统 .active 类）
            const panel = container.closest('[x-show]') || document.getElementById('flamegraph-panel');
            const isVisible = panel && (
                window.getComputedStyle(panel).display !== 'none' ||
                panel.classList.contains('active')
            );
            if (!isVisible) return;
            if (!flameGraphData) return;

            let width = container.clientWidth;
            if (width <= 0) {
                // 如果容器宽度为 0，尝试从父级或 main 元素获取
                const main = document.querySelector('main');
                width = (main ? main.clientWidth : 0) - 40 || 1200;
            }

            // Calculate and display stats
            const stats = calculateFlameStats(flameGraphData);
            document.getElementById('flame-total-samples').textContent = stats.totalSamples.toLocaleString();
            document.getElementById('flame-unique-funcs').textContent = stats.uniqueFuncs.toLocaleString();
            document.getElementById('flame-max-depth').textContent = stats.maxDepth;

            // Create flame graph
            flameChart = flamegraph()
                .width(width)
                .cellHeight(22)
                .transitionDuration(400)
                .minFrameSize(1)
                .transitionEase(d3.easeCubicOut)
                .sort(true)
                .title('')
                .inverted(isInverted)
                .selfValue(false)
                .onClick(this.handleClick.bind(this))
                .tooltip(createFlameTooltip());

            // Color scheme - theme-aware with cool-to-warm gradient for dark mode
            flameChart.setColorMapper(function(d, originalColor) {
                const name = d.data.name || '';
                
                // Check for special function types first
                const style = getComputedStyle(document.documentElement);
                const isDarkMode = document.documentElement.getAttribute('data-theme') === 'dark';
                
                // Check special types and return type-specific colors
                const typeColor = getTypeSpecificColor(name, style);
                if (typeColor) {
                    return typeColor;
                }
                
                // Generate hash from function name for consistent coloring
                let hash = 0;
                for (let i = 0; i < name.length; i++) {
                    hash = ((hash << 5) - hash) + name.charCodeAt(i);
                    hash |= 0;
                }
                
                // Calculate depth ratio for gradient effect (deeper = warmer)
                const depth = d.depth || 0;
                const maxDepth = 50; // Reasonable max depth assumption
                const depthRatio = Math.min(depth / maxDepth, 1);
                
                if (isDarkMode) {
                    // Dark mode: cool-to-warm gradient (blue → purple → pink → amber)
                    // Use depth to influence the gradient position
                    return getDarkModeFlameColor(hash, depthRatio, style);
                } else {
                    // Light mode: classic warm flame colors
                    return getLightModeFlameColor(hash, style);
                }
            });
            
            // Helper function to get type-specific colors
            function getTypeSpecificColor(name, style) {
                // JVM/Java system functions
                if (/^java\.|^javax\.|^jdk\.|^sun\.|^com\.sun\./.test(name)) {
                    const rgb = style.getPropertyValue('--color-flame-jvm').trim() || '255 152 0';
                    return `rgb(${rgb.split(' ').join(', ')})`;
                }
                // GC functions
                if (/GC|Garbage|SafePoint|safepoint|VMThread/i.test(name)) {
                    const rgb = style.getPropertyValue('--color-flame-gc').trim() || '244 67 54';
                    return `rgb(${rgb.split(' ').join(', ')})`;
                }
                // Native functions
                if (/^\[native\]|^libc\.|^libpthread|^__[a-z]|^pthread_|^malloc|^free$|^mmap/.test(name)) {
                    const rgb = style.getPropertyValue('--color-flame-native').trim() || '76 175 80';
                    return `rgb(${rgb.split(' ').join(', ')})`;
                }
                // Kernel functions
                if (/^\[kernel\]|^vmlinux|^do_syscall|^sys_|^__x64_sys|^entry_SYSCALL/.test(name)) {
                    const rgb = style.getPropertyValue('--color-flame-kernel').trim() || '156 39 176';
                    return `rgb(${rgb.split(' ').join(', ')})`;
                }
                // Reflection functions
                if (/reflect|Reflect|\$Proxy|CGLIB|ByteBuddy|MethodHandle|LambdaForm/i.test(name)) {
                    const rgb = style.getPropertyValue('--color-flame-reflect').trim() || '0 188 212';
                    return `rgb(${rgb.split(' ').join(', ')})`;
                }
                return null;
            }
            
            // Dark mode: cool-to-warm gradient
            function getDarkModeFlameColor(hash, depthRatio, style) {
                // Gradient colors from CSS variables
                const gradientColors = [
                    parseRgbVar(style.getPropertyValue('--color-flame-gradient-1').trim()) || [59, 130, 246],   // blue
                    parseRgbVar(style.getPropertyValue('--color-flame-gradient-2').trim()) || [139, 92, 246],  // violet
                    parseRgbVar(style.getPropertyValue('--color-flame-gradient-3').trim()) || [236, 72, 153],  // pink
                    parseRgbVar(style.getPropertyValue('--color-flame-gradient-4').trim()) || [251, 146, 60],  // amber
                ];
                
                // Use hash to add variation, depth to influence gradient position
                const hashVariation = (Math.abs(hash) % 100) / 100; // 0-1
                const gradientPos = depthRatio * 0.6 + hashVariation * 0.4; // Blend depth and hash
                
                // Interpolate between gradient colors
                const color = interpolateGradient(gradientColors, gradientPos);
                
                // Add slight saturation/lightness variation based on hash
                const satAdjust = (Math.abs(hash >> 8) % 20) - 10; // -10 to +10
                const lightAdjust = (Math.abs(hash >> 16) % 15) - 7; // -7 to +7
                
                return adjustColorBrightness(color, satAdjust, lightAdjust);
            }
            
            // Light mode: classic warm flame colors
            function getLightModeFlameColor(hash, style) {
                const baseHue = parseInt(style.getPropertyValue('--color-flame-base-hue')) || 30;
                const hueRange = parseInt(style.getPropertyValue('--color-flame-hue-range')) || 30;
                const satBase = parseInt(style.getPropertyValue('--color-flame-saturation-base')) || 70;
                const lightBase = parseInt(style.getPropertyValue('--color-flame-lightness-base')) || 55;
                
                const hue = baseHue + (Math.abs(hash) % hueRange);
                const saturation = satBase + (Math.abs(hash >> 8) % 25);
                const lightness = lightBase + (Math.abs(hash >> 16) % 15);
                return `hsl(${hue}, ${saturation}%, ${lightness}%)`;
            }
            
            // Parse "r g b" string to [r, g, b] array
            function parseRgbVar(str) {
                if (!str) return null;
                const parts = str.trim().split(/\s+/).map(Number);
                return parts.length === 3 ? parts : null;
            }
            
            // Interpolate between gradient colors
            function interpolateGradient(colors, t) {
                t = Math.max(0, Math.min(1, t));
                const segments = colors.length - 1;
                const segmentT = t * segments;
                const segmentIndex = Math.min(Math.floor(segmentT), segments - 1);
                const localT = segmentT - segmentIndex;
                
                const c1 = colors[segmentIndex];
                const c2 = colors[segmentIndex + 1];
                
                return [
                    Math.round(c1[0] + (c2[0] - c1[0]) * localT),
                    Math.round(c1[1] + (c2[1] - c1[1]) * localT),
                    Math.round(c1[2] + (c2[2] - c1[2]) * localT)
                ];
            }
            
            // Adjust color brightness while keeping it valid
            function adjustColorBrightness(rgb, satAdjust, lightAdjust) {
                // Simple brightness adjustment
                const factor = 1 + lightAdjust / 100;
                const r = Math.min(255, Math.max(0, Math.round(rgb[0] * factor)));
                const g = Math.min(255, Math.max(0, Math.round(rgb[1] * factor)));
                const b = Math.min(255, Math.max(0, Math.round(rgb[2] * factor)));
                return `rgb(${r}, ${g}, ${b})`;
            }

            try {
                d3.select('#flamegraph').datum(flameGraphData).call(flameChart);
            } catch (err) {
                console.error('Error rendering flame graph:', err);
                container.innerHTML = '<div class="loading" style="color: rgb(var(--color-danger));">⚠️ Error rendering flame graph: ' + err.message + '</div>';
                return;
            }

            // Re-apply search after zoom
            d3.select('#flamegraph').on('click', () => {
                container.classList.add('zooming');
                setTimeout(() => {
                    container.classList.remove('zooming');
                    this.reapplySearch();
                }, 420);
            });

            // Handle resize
            let resizeTimeout;
            window.addEventListener('resize', () => {
                clearTimeout(resizeTimeout);
                resizeTimeout = setTimeout(() => {
                    if (flameChart && container.clientWidth > 0) {
                        flameChart.width(container.clientWidth);
                        d3.select('#flamegraph').datum(flameGraphData).call(flameChart);
                    }
                }, 200);
            });
        },

        handleClick(d) {
            const container = document.getElementById('flamegraph');
            container.classList.add('zooming');
        },

        search() {
            const term = document.getElementById('searchInput').value.trim();
            const badge = document.getElementById('searchResultBadge');

            if (!flameChart || !term) {
                badge.classList.add('hidden');
                this.clearSearch();
                return;
            }

            currentSearchTerm = term;
            const container = document.getElementById('flamegraph');
            container.classList.add('searching');

            flameChart.search(term);
            const matchCount = applySearchHighlight(term);

            badge.classList.remove('hidden', 'no-match');
            if (matchCount > 0) {
                badge.innerHTML = `✓ Found ${matchCount} matching frame${matchCount > 1 ? 's' : ''}`;
                badge.style.background = 'rgb(var(--color-success))';
            } else {
                badge.classList.add('no-match');
                badge.innerHTML = `✗ No matches for "${Utils.escapeHtml(term)}"`;
                badge.style.background = 'rgb(var(--color-danger))';
            }
        },

        reapplySearch() {
            if (currentSearchTerm) {
                const container = document.getElementById('flamegraph');
                container.classList.add('searching');
                requestAnimationFrame(() => {
                    const matchCount = applySearchHighlight(currentSearchTerm);
                    const badge = document.getElementById('searchResultBadge');
                    badge.classList.remove('hidden', 'no-match');
                    if (matchCount > 0) {
                        badge.innerHTML = `✓ Found ${matchCount} matching frame${matchCount > 1 ? 's' : ''}`;
                        badge.style.background = 'rgb(var(--color-success))';
                    } else {
                        badge.classList.add('no-match');
                        badge.innerHTML = `✗ No matches in current view`;
                        badge.style.background = 'rgb(var(--color-danger))';
                    }
                });
            }
        },

        clearSearch() {
            document.getElementById('searchInput').value = '';
            document.getElementById('searchResultBadge').classList.add('hidden');
            currentSearchTerm = '';
            const container = document.getElementById('flamegraph');
            container.classList.remove('searching');
            d3.select('#flamegraph').selectAll('.d3-flame-graph g.search-match')
                .classed('search-match', false);
            if (flameChart) flameChart.clear();
        },

        reset() {
            if (flameChart) {
                const container = document.getElementById('flamegraph');
                container.classList.add('zooming');
                flameChart.resetZoom();
                setTimeout(() => {
                    container.classList.remove('zooming');
                    this.reapplySearch();
                }, 420);
            }
        },

        toggleFilter(filterType) {
            const chip = document.querySelector(`#flameFilterSection .filter-chip[data-filter="${filterType}"]`);
            if (flameFilters.has(filterType)) {
                flameFilters.delete(filterType);
                chip.classList.remove('active');
            } else {
                flameFilters.add(filterType);
                chip.classList.add('active');
            }
            this.applyFilters();
        },

        applyFilters() {
            if (!originalFlameGraphData) return;
            const container = document.getElementById('flamegraph');

            if (flameFilters.size === 0) {
                container.classList.remove('filter-system');
                flameGraphData = deepCloneFlameData(originalFlameGraphData);
            } else {
                container.classList.add('filter-system');
                flameGraphData = collapseFilteredNodes(originalFlameGraphData, flameFilters);
            }

            if (flameGraphData) {
                container.innerHTML = '';
                flameChart = null;
                validateFlameData(flameGraphData);

                const hasChildren = flameGraphData.children && flameGraphData.children.length > 0;
                const hasValue = flameGraphData.value > 0;

                if (!hasValue && !hasChildren) {
                    container.innerHTML = '<div class="loading" style="color: rgb(var(--color-danger));">⚠️ All functions were filtered out. Try removing some filters.</div>';
                    return;
                }

                this.render();
                if (currentSearchTerm) {
                    setTimeout(() => this.search(), 100);
                }
            }
        },

        searchFor(funcName) {
            const searchTerm = Utils.extractSearchTerm(funcName);
            document.getElementById('searchInput').value = searchTerm;
            setTimeout(() => this.search(), 100);
        },

        getData() {
            return flameGraphData;
        },

        // Thread selector methods
        updateThreadSelector() {
            const container = document.getElementById('flameThreadSelectorContainer');
            const countBadge = document.getElementById('flameThreadCount');
            const dropdown = document.getElementById('flameThreadDropdown');
            const selectedText = document.getElementById('flameThreadSelectedText');

            if (!container) return;

            if (!hasThreadData || threadFlameGraphs.length === 0) {
                container.style.display = 'none';
                return;
            }

            container.style.display = 'flex';

            if (countBadge) {
                countBadge.textContent = threadFlameGraphs.length;
            }

            // Render dropdown items
            this.renderThreadDropdownItems('');
        },

        renderThreadDropdownItems(filter) {
            const dropdown = document.getElementById('flameThreadDropdown');
            if (!dropdown) return;

            const filterLower = (filter || '').toLowerCase();
            let html = '';

            // Global view option (always show)
            if (!filter || 'global'.includes(filterLower) || 'all threads'.includes(filterLower)) {
                const isSelected = selectedThreadTid === null;
                html += `<div class="thread-dropdown-item global-item ${isSelected ? 'selected' : ''}" data-tid="" onclick="selectFlameThread('')">
                    <span class="item-icon">🌐</span>
                    <span class="item-text">Global View (All Threads)</span>
                </div>`;
            }

            // Filter threads
            const filteredThreads = threadFlameGraphs.filter(t => {
                if (!filter) return true;
                const name = (t.name || `Thread-${t.tid}`).toLowerCase();
                const group = (t.group || '').toLowerCase();
                const tid = String(t.tid);
                return name.includes(filterLower) || group.includes(filterLower) || tid.includes(filterLower);
            });

            // Render thread items
            filteredThreads.forEach(t => {
                const pct = t.percentage ? t.percentage.toFixed(1) : '0.0';
                const displayName = t.name || `Thread-${t.tid}`;
                const groupInfo = t.group ? `[${t.group}]` : '';
                const samples = t.samples ? t.samples.toLocaleString() : '0';
                const isSelected = selectedThreadTid === t.tid;

                html += `<div class="thread-dropdown-item ${isSelected ? 'selected' : ''}" data-tid="${t.tid}" 
                             onclick="selectFlameThread('${t.tid}')" 
                             title="TID: ${t.tid}, Samples: ${samples}">
                    <div class="item-main">
                        <span class="item-icon">🧵</span>
                        <span class="item-text">${Utils.escapeHtml(displayName)}</span>
                        ${groupInfo ? `<span class="item-group">${Utils.escapeHtml(groupInfo)}</span>` : ''}
                    </div>
                    <span class="item-pct">${pct}%</span>
                </div>`;
            });

            if (filteredThreads.length === 0 && filter) {
                html += `<div class="thread-dropdown-empty">No threads match "${Utils.escapeHtml(filter)}"</div>`;
            }

            dropdown.innerHTML = html;
        },

        toggleThreadDropdown(show) {
            const wrapper = document.getElementById('flameThreadSelectorWrapper');
            const input = document.getElementById('flameThreadSearchInput');
            if (!wrapper) return;

            if (show === undefined) {
                show = !wrapper.classList.contains('open');
            }

            if (show) {
                wrapper.classList.add('open');
                if (input) {
                    input.value = '';
                    input.focus();
                }
                this.renderThreadDropdownItems('');

                // Close dropdown when clicking outside
                setTimeout(() => {
                    document.addEventListener('click', this._closeDropdownHandler = (e) => {
                        if (!wrapper.contains(e.target)) {
                            this.toggleThreadDropdown(false);
                        }
                    });
                }, 0);
            } else {
                wrapper.classList.remove('open');
                if (this._closeDropdownHandler) {
                    document.removeEventListener('click', this._closeDropdownHandler);
                    this._closeDropdownHandler = null;
                }
            }
        },

        handleThreadSearch(value) {
            this.renderThreadDropdownItems(value);
        },

        handleThreadSearchKeydown(event) {
            if (event.key === 'Escape') {
                this.toggleThreadDropdown(false);
            } else if (event.key === 'Enter') {
                // Select first visible item
                const dropdown = document.getElementById('flameThreadDropdown');
                const firstItem = dropdown?.querySelector('.thread-dropdown-item');
                if (firstItem) {
                    const tid = firstItem.getAttribute('data-tid');
                    this.selectThread(tid);
                }
            }
        },

        selectThread(tid) {
            const selectedText = document.getElementById('flameThreadSelectedText');

            if (tid === '' || tid === null || tid === undefined) {
                // Switch to global view
                selectedThreadTid = null;
                if (selectedText) {
                    selectedText.innerHTML = '<span class="global-icon">🌐</span> Global View (All Threads)';
                }
                this.toggleThreadDropdown(false);

                // Restore global flame graph
                if (originalApiData) {
                    flameGraphData = transformFlameData(originalApiData);
                    originalFlameGraphData = deepCloneFlameData(flameGraphData);
                    flameFilters.clear();
                    this.clearFiltersUI();
                    this.clearSearch();
                    this.render();
                }
            } else {
                // Switch to specific thread
                selectedThreadTid = parseInt(tid);
                const thread = threadFlameGraphs.find(t => t.tid === selectedThreadTid);

                if (thread && thread.flame_root) {
                    const displayName = thread.name || `Thread-${thread.tid}`;
                    const pct = thread.percentage ? thread.percentage.toFixed(1) : '0.0';

                    if (selectedText) {
                        selectedText.innerHTML = `<span class="thread-icon">🧵</span> ${Utils.escapeHtml(displayName)} <span class="selected-pct">(${pct}%)</span>`;
                    }
                    this.toggleThreadDropdown(false);

                    // Render thread-specific flame graph
                    flameGraphData = transformFlameData({ root: thread.flame_root });
                    originalFlameGraphData = deepCloneFlameData(flameGraphData);
                    flameFilters.clear();
                    this.clearFiltersUI();
                    this.clearSearch();
                    this.render();
                }
            }
        },

        clearFiltersUI() {
            const chips = document.querySelectorAll('#flameFilterSection .filter-chip');
            chips.forEach(chip => chip.classList.remove('active'));
        },

        // Check if thread data is available
        hasThreads() {
            return hasThreadData;
        },

        // Get current selected thread
        getSelectedThread() {
            return selectedThreadTid;
        },

        // Get top functions from the loaded flame graph data
        getTopFunctions() {
            if (!originalApiData || !originalApiData.thread_analysis) {
                return [];
            }
            return originalApiData.thread_analysis.top_functions || [];
        },

        // Get total samples from the loaded flame graph data
        getTotalSamples() {
            if (!originalApiData) {
                return 0;
            }
            return originalApiData.total_samples || 0;
        }
    };
})();

// Export for global access
window.FlameGraph = FlameGraph;

// Listen for theme changes to re-render flame graph with new colors
if (typeof ThemeManager !== 'undefined') {
    ThemeManager.onChange(function(themeId) {
        // Re-render flame graph when theme changes
        // Check if flame graph is visible and has data
        const container = document.getElementById('flamegraph');
        const panel = container?.closest('[x-show]') || document.getElementById('flamegraph-panel');
        const isVisible = panel && (
            window.getComputedStyle(panel).display !== 'none' ||
            panel.classList.contains('active')
        );
        
        if (isVisible && FlameGraph.getData()) {
            console.log('[FlameGraph] Theme changed to:', themeId, '- re-rendering');
            setTimeout(() => FlameGraph.render(), 100);
        }
    });
}
