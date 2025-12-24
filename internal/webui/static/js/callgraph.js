/**
 * Call Graph Module
 * Handles call graph rendering, search, and filtering
 */

const CallGraph = (function() {
    // Private state
    let svg = null;
    let zoom = null;
    let simulation = null;
    let nodes = [];
    let links = [];
    let nodeSelection = null;
    let linkSelection = null;
    let originalData = null;

    // Search state
    let searchMatches = [];
    let searchIndex = 0;
    let searchTerm = '';
    let viewMode = 'all';
    let callersMap = null;
    let calleesMap = null;
    let chainSet = new Set();
    let rootSet = new Set();
    let chainEdges = new Set();
    let nodeDepths = new Map();

    // Filter state
    let filters = new Set();
    let filteredNodeIndices = new Set();
    let bridgeEdges = [];
    let bridgeEdgeSelection = null;
    let linkContainer = null;

    // Thread selector state
    let threadCallGraphs = [];
    let selectedThreadTid = null; // null means global view
    let hasThreadData = false;

    // Performance limits
    const MAX_MATCHES = 200;
    const MAX_CHAIN_DEPTH = 50;
    const MAX_CHAIN_NODES = 2000;

    // System function patterns (shared with FlameGraph)
    const SYSTEM_PATTERNS = {
        jvm: [/^java\./, /^javax\./, /^jdk\./, /^sun\./, /^com\.sun\./, /^org\.openjdk\./, /Unsafe\./, /^jdk\.internal\./],
        gc: [/GC/i, /Garbage/i, /^G1/, /^ZGC/, /^Shenandoah/, /ParallelGC/, /ConcurrentMark/, /SafePoint/i, /VMThread/],
        native: [/^\[native\]/, /^libc\./, /^libpthread/, /^__/, /^_Z/, /^std::/, /^pthread_/, /^malloc/, /^free$/, /^syscall/],
        kernel: [/^\[kernel\]/, /^vmlinux/, /^do_syscall/, /^sys_/, /^__x64_sys/, /^entry_SYSCALL/, /^__schedule/, /^schedule$/],
        reflect: [/reflect/i, /Reflect/, /\$Proxy/, /CGLIB/, /ByteBuddy/, /javassist/, /MethodHandle/, /LambdaForm/]
    };

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

    // Configuration for multi-line node display
    // Prioritizes showing complete method names for better readability
    const NODE_CONFIG = {
        maxLineWidth: 28,      // Max characters per line (shorter for better fit)
        maxLines: 3,           // Max number of lines
        charWidth: 6.5,        // Approximate character width in pixels
        lineHeight: 13,        // Line height in pixels
        paddingX: 12,          // Horizontal padding
        paddingY: 6,           // Vertical padding
        minWidth: 80,
        maxWidth: 220          // Smaller max width for better graph layout
    };

    // Position tooltip near cursor, avoiding edge overflow
    function positionTooltip(tooltip, event) {
        const padding = 15;
        const viewportWidth = window.innerWidth;
        const viewportHeight = window.innerHeight;
        const tooltipRect = tooltip.getBoundingClientRect();
        
        let left = event.pageX + padding;
        let top = event.pageY + padding;
        
        // Avoid right edge overflow
        if (left + tooltipRect.width > viewportWidth - padding) {
            left = event.pageX - tooltipRect.width - padding;
        }
        // Avoid bottom edge overflow
        if (top + tooltipRect.height > viewportHeight - padding) {
            top = event.pageY - tooltipRect.height - padding;
        }
        
        tooltip.style.left = left + 'px';
        tooltip.style.top = top + 'px';
    }

    // Build enhanced tooltip HTML for a node
    function buildNodeTooltip(d) {
        const parsed = Utils.parseMethodName(d.name);
        const formatted = Utils.formatFunctionName(d.name, { showBadge: false });
        const selfPct = d.selfPct || 0;
        const totalPct = d.totalPct || 0;
        
        // Get callers and callees count
        const callerCount = callersMap ? (callersMap.get(d.index) || []).length : 0;
        const calleeCount = calleesMap ? (calleesMap.get(d.index) || []).length : 0;
        
        let html = '<div class="cg-tooltip">';
        
        // Header with function name hierarchy
        html += '<div class="cg-tooltip-header">';
        
        // Package/Namespace (if exists)
        if (parsed.package) {
            const pkgDisplay = typeof parsed.package === 'string' 
                ? parsed.package 
                : parsed.package.join('.');
            html += `<div class="cg-tooltip-pkg">📦 ${Utils.escapeHtml(pkgDisplay)}</div>`;
        }
        
        // Class (if exists)
        if (parsed.class) {
            html += `<div class="cg-tooltip-class">🔷 ${Utils.escapeHtml(parsed.class)}</div>`;
        }
        
        // Method name (main)
        const methodName = parsed.method || formatted.displayName;
        html += `<div class="cg-tooltip-method">⚡ ${Utils.escapeHtml(methodName)}</div>`;
        
        html += '</div>';
        
        // Allocation badge (if applicable)
        if (formatted.isAllocation) {
            const allocIcon = formatted.allocationType === 'instance' ? '📦' : '📊';
            const allocLabel = formatted.allocationType === 'instance' 
                ? 'Instance Allocation' 
                : 'Size Allocation';
            html += `<div class="cg-tooltip-alloc">${allocIcon} ${allocLabel}</div>`;
        }
        
        // Performance metrics
        html += '<div class="cg-tooltip-metrics">';
        
        // Self percentage with progress bar
        html += '<div class="cg-tooltip-metric">';
        html += '<div class="cg-tooltip-metric-row">';
        html += '<span class="cg-tooltip-label">Self</span>';
        html += `<span class="cg-tooltip-value cg-tooltip-self">${selfPct.toFixed(2)}%</span>`;
        html += '</div>';
        html += `<div class="cg-tooltip-bar"><div class="cg-tooltip-bar-fill cg-tooltip-bar-self" style="width:${Math.min(100, selfPct)}%"></div></div>`;
        html += '</div>';
        
        // Total percentage with progress bar
        html += '<div class="cg-tooltip-metric">';
        html += '<div class="cg-tooltip-metric-row">';
        html += '<span class="cg-tooltip-label">Total</span>';
        html += `<span class="cg-tooltip-value cg-tooltip-total">${totalPct.toFixed(2)}%</span>`;
        html += '</div>';
        html += `<div class="cg-tooltip-bar"><div class="cg-tooltip-bar-fill cg-tooltip-bar-total" style="width:${Math.min(100, totalPct)}%"></div></div>`;
        html += '</div>';
        
        html += '</div>';
        
        // Call relationship info
        html += '<div class="cg-tooltip-calls">';
        html += `<span class="cg-tooltip-callers" title="Callers (who calls this function)">⬆️ ${callerCount} caller${callerCount !== 1 ? 's' : ''}</span>`;
        html += `<span class="cg-tooltip-callees" title="Callees (functions called by this)">⬇️ ${calleeCount} callee${calleeCount !== 1 ? 's' : ''}</span>`;
        html += '</div>';
        
        // Hint
        html += '<div class="cg-tooltip-hint">Click to focus · Double-click for details</div>';
        
        html += '</div>';
        return html;
    }

    // Detect the type of function name and parse accordingly
    function parseMethodName(name) {
        if (!name) return { type: 'simple', parts: [''] };
        
        // Type 1: File path with function (e.g., /usr/lib/xxx/libc.so.6 or libc.so.6::malloc)
        if (name.includes('/')) {
            // Extract filename from path
            const pathParts = name.split('/');
            const filename = pathParts[pathParts.length - 1] || pathParts[pathParts.length - 2] || name;
            
            // Check if there's a function after :: 
            if (filename.includes('::')) {
                const [lib, func] = filename.split('::');
                return { 
                    type: 'native_lib', 
                    library: lib,
                    method: func,
                    path: pathParts.slice(0, -1).join('/')
                };
            }
            
            return { 
                type: 'path', 
                filename: filename,
                path: pathParts.slice(0, -1).join('/')
            };
        }
        
        // Type 2: C++ style with :: (e.g., Namespace::Class::method)
        if (name.includes('::')) {
            const parts = name.split('::');
            if (parts.length >= 2) {
                const method = parts[parts.length - 1];
                const classOrNs = parts.slice(0, -1).join('::');
                return {
                    type: 'cpp',
                    namespace: classOrNs,
                    method: method
                };
            }
        }
        
        // Type 3: Java style with . (e.g., com.example.Class.method)
        if (name.includes('.')) {
            const parts = name.split('.');
            if (parts.length >= 2) {
                const method = parts[parts.length - 1];
                const className = parts[parts.length - 2];
                const packageParts = parts.slice(0, -2);
                return {
                    type: 'java',
                    package: packageParts,
                    className: className,
                    method: method
                };
            }
        }
        
        // Type 4: Simple name
        return { type: 'simple', name: name };
    }

    // Split name into multiple lines for display
    // Strategy: Show most important info (class + method) prominently
    function getMultiLineDisplayName(name) {
        if (!name) return { lines: [''], width: NODE_CONFIG.minWidth };
        
        const cleanName = Utils.stripAllocationMarker(name);
        const parsed = parseMethodName(cleanName);
        const maxLen = NODE_CONFIG.maxLineWidth;
        
        let lines = [];
        let maxLineLen = 0;
        
        switch (parsed.type) {
            case 'native_lib': {
                // Line 1: Path (abbreviated)
                if (parsed.path) {
                    let pathDisplay = parsed.path;
                    if (pathDisplay.length > maxLen) {
                        // Show only last directory
                        const dirs = parsed.path.split('/').filter(d => d);
                        pathDisplay = '…/' + (dirs[dirs.length - 1] || '');
                    }
                    if (pathDisplay.length > maxLen) {
                        pathDisplay = pathDisplay.substring(0, maxLen - 1) + '…';
                    }
                    lines.push(pathDisplay);
                    maxLineLen = Math.max(maxLineLen, pathDisplay.length);
                }
                
                // Line 2: Library name
                let libDisplay = parsed.library;
                if (libDisplay.length > maxLen) {
                    libDisplay = libDisplay.substring(0, maxLen - 1) + '…';
                }
                lines.push(libDisplay);
                maxLineLen = Math.max(maxLineLen, libDisplay.length);
                
                // Line 3: Function name
                if (parsed.method) {
                    let methodDisplay = '→ ' + parsed.method;
                    if (methodDisplay.length > maxLen) {
                        methodDisplay = '→ ' + parsed.method.substring(0, maxLen - 3) + '…';
                    }
                    lines.push(methodDisplay);
                    maxLineLen = Math.max(maxLineLen, methodDisplay.length);
                }
                break;
            }
            
            case 'path': {
                // Line 1: Path (abbreviated)
                if (parsed.path) {
                    let pathDisplay = parsed.path;
                    if (pathDisplay.length > maxLen) {
                        const dirs = parsed.path.split('/').filter(d => d);
                        pathDisplay = '…/' + (dirs[dirs.length - 1] || '');
                    }
                    if (pathDisplay.length > maxLen) {
                        pathDisplay = pathDisplay.substring(0, maxLen - 1) + '…';
                    }
                    lines.push(pathDisplay);
                    maxLineLen = Math.max(maxLineLen, pathDisplay.length);
                }
                
                // Line 2: Filename
                let fileDisplay = parsed.filename;
                if (fileDisplay.length > maxLen) {
                    fileDisplay = fileDisplay.substring(0, maxLen - 1) + '…';
                }
                lines.push(fileDisplay);
                maxLineLen = Math.max(maxLineLen, fileDisplay.length);
                break;
            }
            
            case 'cpp': {
                // Line 1: Namespace/class (abbreviated)
                let nsDisplay = parsed.namespace;
                if (nsDisplay.length > maxLen) {
                    // Try to abbreviate
                    const nsParts = nsDisplay.split('::');
                    if (nsParts.length > 1) {
                        nsDisplay = nsParts.map((p, i) => i < nsParts.length - 1 ? p.substring(0, 3) : p).join('::');
                    }
                }
                if (nsDisplay.length > maxLen) {
                    nsDisplay = nsDisplay.substring(0, maxLen - 1) + '…';
                }
                lines.push(nsDisplay);
                maxLineLen = Math.max(maxLineLen, nsDisplay.length);
                
                // Line 2: Method name
                let methodDisplay = '→ ' + parsed.method;
                if (methodDisplay.length > maxLen) {
                    methodDisplay = '→ ' + parsed.method.substring(0, maxLen - 3) + '…';
                }
                lines.push(methodDisplay);
                maxLineLen = Math.max(maxLineLen, methodDisplay.length);
                break;
            }
            
            case 'java': {
                // Line 1: Abbreviated package (if exists)
                if (parsed.package.length > 0) {
                    let pkgDisplay;
                    if (parsed.package.length <= 2) {
                        pkgDisplay = parsed.package.join('.');
                    } else {
                        // Abbreviate: io.netty.channel -> i.n.channel
                        pkgDisplay = parsed.package.slice(0, -1).map(p => p.charAt(0)).join('.') + '.' + parsed.package[parsed.package.length - 1];
                    }
                    
                    if (pkgDisplay.length > maxLen) {
                        pkgDisplay = parsed.package.map(p => p.charAt(0)).join('.');
                    }
                    if (pkgDisplay.length > maxLen) {
                        pkgDisplay = pkgDisplay.substring(0, maxLen - 1) + '…';
                    }
                    
                    lines.push(pkgDisplay);
                    maxLineLen = Math.max(maxLineLen, pkgDisplay.length);
                }
                
                // Line 2: Class name
                let classDisplay = parsed.className;
                if (classDisplay.length > maxLen) {
                    classDisplay = classDisplay.substring(0, maxLen - 1) + '…';
                }
                lines.push(classDisplay);
                maxLineLen = Math.max(maxLineLen, classDisplay.length);
                
                // Line 3: Method name
                let methodDisplay = '→ ' + parsed.method;
                if (methodDisplay.length > maxLen) {
                    methodDisplay = '→ ' + parsed.method.substring(0, maxLen - 3) + '…';
                }
                lines.push(methodDisplay);
                maxLineLen = Math.max(maxLineLen, methodDisplay.length);
                break;
            }
            
            default: {
                // Simple name - try to fit or split
                let displayName = parsed.name || cleanName;
                
                if (displayName.length <= maxLen) {
                    lines.push(displayName);
                    maxLineLen = displayName.length;
                } else {
                    // Split long names across lines
                    let remaining = displayName;
                    while (remaining.length > 0 && lines.length < NODE_CONFIG.maxLines) {
                        if (remaining.length <= maxLen) {
                            lines.push(remaining);
                            maxLineLen = Math.max(maxLineLen, remaining.length);
                            break;
                        }
                        // Try to break at reasonable points
                        let breakPoint = maxLen;
                        const breakChars = ['_', ':', '<', '>', '(', '-'];
                        for (let i = maxLen - 1; i > maxLen - 10 && i > 0; i--) {
                            if (breakChars.includes(remaining[i])) {
                                breakPoint = i + 1;
                                break;
                            }
                        }
                        const line = remaining.substring(0, breakPoint);
                        lines.push(line);
                        maxLineLen = Math.max(maxLineLen, line.length);
                        remaining = remaining.substring(breakPoint);
                    }
                    if (remaining.length > 0 && lines.length > 0) {
                        const lastLine = lines[lines.length - 1];
                        if (lastLine.length >= maxLen - 1) {
                            lines[lines.length - 1] = lastLine.substring(0, maxLen - 1) + '…';
                        } else {
                            lines[lines.length - 1] = lastLine + '…';
                        }
                    }
                }
                break;
            }
        }
        
        // Calculate width based on longest line
        const width = Math.min(
            NODE_CONFIG.maxWidth,
            Math.max(NODE_CONFIG.minWidth, maxLineLen * NODE_CONFIG.charWidth + NODE_CONFIG.paddingX * 2)
        );
        
        return { lines, width };
    }

    function getNodeWidth(name) {
        return getMultiLineDisplayName(name).width;
    }

    function getNodeHeight(name) {
        const { lines } = getMultiLineDisplayName(name);
        return lines.length * NODE_CONFIG.lineHeight + NODE_CONFIG.paddingY * 2;
    }

    // Internal function for display name calculation (used by getNodeWidth)
    function getDisplayNameInternal(cleanName, maxLen) {
        if (!cleanName) return '';
        const parts = cleanName.split('.');
        if (parts.length >= 2) {
            const className = parts[parts.length - 2] || '';
            const methodName = parts[parts.length - 1] || '';
            const combined = className + '::' + methodName;
            if (combined.length <= maxLen) return combined;
            // Try shorter class name
            const shortClass = className.length > 20 ? className.substring(0, 18) + '..' : className;
            const shortCombined = shortClass + '::' + methodName;
            if (shortCombined.length <= maxLen) return shortCombined;
            if (methodName.length <= maxLen) return methodName;
            return methodName.substring(0, maxLen - 2) + '..';
        }
        if (cleanName.length <= maxLen) return cleanName;
        return cleanName.substring(0, maxLen - 2) + '..';
    }

    function getDisplayName(name, maxLen) {
        if (!name) return '';
        // Strip allocation marker for display
        const cleanName = Utils.stripAllocationMarker(name);
        return getDisplayNameInternal(cleanName, maxLen);
    }

    function getShortName(name) {
        if (!name) return '';
        // Strip allocation marker for display
        const cleanName = Utils.stripAllocationMarker(name);
        const parts = cleanName.split('.');
        if (parts.length >= 2) {
            return parts.slice(-2).join('.');
        }
        return cleanName.length > 40 ? cleanName.substring(0, 38) + '..' : cleanName;
    }

    function getNodeBoundaryPoint(cx, cy, halfWidth, halfHeight, angle) {
        const tanAngle = Math.tan(angle);
        let x, y;
        if (Math.abs(Math.cos(angle)) > 0.001) {
            const signX = Math.cos(angle) > 0 ? 1 : -1;
            x = cx + signX * halfWidth;
            y = cy + signX * halfWidth * tanAngle;
            if (Math.abs(y - cy) <= halfHeight) {
                return { x, y };
            }
        }
        const signY = Math.sin(angle) > 0 ? 1 : -1;
        y = cy + signY * halfHeight;
        x = cx + signY * halfHeight / tanAngle;
        return { x, y };
    }

    function calculateEdgePath(d) {
        const sourceX = d.source.x;
        const sourceY = d.source.y;
        const targetX = d.target.x;
        const targetY = d.target.y;
        const dx = targetX - sourceX;
        const dy = targetY - sourceY;
        const angle = Math.atan2(dy, dx);
        const sourceHalfWidth = getNodeWidth(d.source.name) / 2;
        const targetHalfWidth = getNodeWidth(d.target.name) / 2;
        const sourceHalfHeight = getNodeHeight(d.source.name) / 2;
        const targetHalfHeight = getNodeHeight(d.target.name) / 2;
        const sourceIntersect = getNodeBoundaryPoint(sourceX, sourceY, sourceHalfWidth, sourceHalfHeight, angle);
        const targetIntersect = getNodeBoundaryPoint(targetX, targetY, targetHalfWidth, targetHalfHeight, angle + Math.PI);
        return `M ${sourceIntersect.x},${sourceIntersect.y} L ${targetIntersect.x},${targetIntersect.y}`;
    }

    function buildAdjacencyMaps() {
        callersMap = new Map();
        calleesMap = new Map();
        links.forEach(l => {
            const sourceIdx = l.source.index;
            const targetIdx = l.target.index;
            if (!callersMap.has(targetIdx)) callersMap.set(targetIdx, []);
            callersMap.get(targetIdx).push(sourceIdx);
            if (!calleesMap.has(sourceIdx)) calleesMap.set(sourceIdx, []);
            calleesMap.get(sourceIdx).push(targetIdx);
        });
    }

    function analyzeCallChains() {
        chainSet = new Set();
        rootSet = new Set();
        chainEdges = new Set();
        nodeDepths = new Map();

        const matchSet = new Set(searchMatches.map(n => n.index));
        const globalVisited = new Set();
        const matchesToProcess = searchMatches.slice(0, MAX_MATCHES);

        matchesToProcess.forEach(matchNode => {
            const queue = [[matchNode.index, 0, matchNode.index]];
            const localVisited = new Set();

            while (queue.length > 0 && chainSet.size < MAX_CHAIN_NODES) {
                const [nodeIndex, depth, prevNonFilteredIdx] = queue.shift();
                if (localVisited.has(nodeIndex) || Math.abs(depth) > MAX_CHAIN_DEPTH) continue;
                localVisited.add(nodeIndex);

                const isFiltered = filteredNodeIndices.has(nodeIndex);

                if (!isFiltered) {
                    chainSet.add(nodeIndex);
                    const existingDepth = nodeDepths.get(nodeIndex);
                    if (existingDepth === undefined || Math.abs(depth) < Math.abs(existingDepth)) {
                        nodeDepths.set(nodeIndex, depth);
                    }
                    if (prevNonFilteredIdx !== nodeIndex) {
                        chainEdges.add(`${nodeIndex}-${prevNonFilteredIdx}`);
                    }
                }

                const callers = callersMap.get(nodeIndex) || [];
                const nonFilteredCallers = callers.filter(idx => !filteredNodeIndices.has(idx));

                if (callers.length === 0 || (nonFilteredCallers.length === 0 && !isFiltered)) {
                    if (!isFiltered) rootSet.add(nodeIndex);
                }

                callers.forEach(callerIdx => {
                    if (!globalVisited.has(callerIdx) && !localVisited.has(callerIdx)) {
                        const nextPrevNonFiltered = isFiltered ? prevNonFilteredIdx : nodeIndex;
                        queue.push([callerIdx, depth - 1, nextPrevNonFiltered]);
                    }
                });
            }
            localVisited.forEach(idx => globalVisited.add(idx));
        });
    }

    function applySearchHighlight() {
        if (!nodeSelection || !linkSelection) return;

        const matchSet = new Set(searchMatches.map(n => n.index));
        const currentNode = searchMatches[searchIndex];
        const visibleNodes = new Set([...matchSet, ...chainSet]);
        
        // Get callees of current match for highlighting
        let calleeSet = new Set();
        let calleeEdges = new Set();
        if (currentNode && calleesMap) {
            const calleeIndices = calleesMap.get(currentNode.index) || [];
            calleeIndices.forEach(idx => {
                calleeSet.add(idx);
                calleeEdges.add(`${currentNode.index}-${idx}`);
                visibleNodes.add(idx);
            });
        }

        nodeSelection
            .classed('search-match', d => matchSet.has(d.index) && !filteredNodeIndices.has(d.index))
            .classed('current-focus', d => currentNode && d.index === currentNode.index && !filteredNodeIndices.has(d.index))
            .classed('search-chain', d => chainSet.has(d.index) && !matchSet.has(d.index) && !rootSet.has(d.index) && !calleeSet.has(d.index) && !filteredNodeIndices.has(d.index))
            .classed('search-root', d => rootSet.has(d.index) && !matchSet.has(d.index) && !filteredNodeIndices.has(d.index))
            .classed('search-callee', d => calleeSet.has(d.index) && !matchSet.has(d.index) && !filteredNodeIndices.has(d.index))
            .classed('search-dimmed', d => searchTerm && !visibleNodes.has(d.index) && !filteredNodeIndices.has(d.index))
            .classed('search-hidden', d => viewMode === 'focus' && searchTerm && !visibleNodes.has(d.index) && !filteredNodeIndices.has(d.index));

        linkSelection
            .classed('search-chain', l => chainEdges.has(`${l.source.index}-${l.target.index}`) && !filteredNodeIndices.has(l.source.index) && !filteredNodeIndices.has(l.target.index))
            .classed('search-callee-edge', l => calleeEdges.has(`${l.source.index}-${l.target.index}`) && !filteredNodeIndices.has(l.source.index) && !filteredNodeIndices.has(l.target.index))
            .classed('search-connected', l => (matchSet.has(l.source.index) || matchSet.has(l.target.index)) && !chainEdges.has(`${l.source.index}-${l.target.index}`) && !calleeEdges.has(`${l.source.index}-${l.target.index}`) && !filteredNodeIndices.has(l.source.index) && !filteredNodeIndices.has(l.target.index))
            .classed('search-dimmed', l => {
                if (!searchTerm) return false;
                if (filteredNodeIndices.has(l.source.index) || filteredNodeIndices.has(l.target.index)) return false;
                const isChain = chainEdges.has(`${l.source.index}-${l.target.index}`);
                const isCallee = calleeEdges.has(`${l.source.index}-${l.target.index}`);
                const isConnected = matchSet.has(l.source.index) || matchSet.has(l.target.index);
                return !isChain && !isCallee && !isConnected;
            })
            .classed('search-hidden', l => {
                if (viewMode !== 'focus' || !searchTerm) return false;
                if (filteredNodeIndices.has(l.source.index) || filteredNodeIndices.has(l.target.index)) return false;
                return !visibleNodes.has(l.source.index) || !visibleNodes.has(l.target.index);
            })
            .attr('marker-end', l => {
                if (calleeEdges.has(`${l.source.index}-${l.target.index}`)) return 'url(#arrowhead-callee)';
                if (chainEdges.has(`${l.source.index}-${l.target.index}`)) return 'url(#arrowhead-chain)';
                if (matchSet.has(l.source.index) || matchSet.has(l.target.index)) return 'url(#arrowhead-highlight)';
                return 'url(#arrowhead)';
            });

        if (bridgeEdgeSelection) {
            bridgeEdgeSelection.classed('search-dimmed', d => {
                if (!searchTerm) return false;
                const isChainEdge = chainEdges.has(`${d.source.index}-${d.target.index}`);
                const isCalleeEdge = calleeEdges.has(`${d.source.index}-${d.target.index}`);
                const isConnected = matchSet.has(d.source.index) || matchSet.has(d.target.index);
                return !isChainEdge && !isCalleeEdge && !isConnected;
            });
            bridgeEdgeSelection.classed('search-hidden', d => {
                if (viewMode !== 'focus' || !searchTerm) return false;
                const bothVisible = visibleNodes.has(d.source.index) && visibleNodes.has(d.target.index);
                if (!bothVisible) return true;
                return !chainEdges.has(`${d.source.index}-${d.target.index}`);
            });
        }
    }

    function updateSearchUI() {
        const counter = document.getElementById('cgSearchCounter');
        const prevBtn = document.getElementById('cgPrevBtn');
        const nextBtn = document.getElementById('cgNextBtn');

        if (searchMatches.length > 0) {
            counter.textContent = `${searchIndex + 1} / ${searchMatches.length}`;
            counter.style.color = 'rgb(var(--color-success))';
            prevBtn.disabled = false;
            nextBtn.disabled = false;
        } else if (searchTerm) {
            counter.textContent = 'No match';
            counter.style.color = 'rgb(var(--color-danger))';
            prevBtn.disabled = true;
            nextBtn.disabled = true;
        } else {
            counter.textContent = '';
            prevBtn.disabled = true;
            nextBtn.disabled = true;
        }
    }

    function updateStats() {
        document.getElementById('cgStatMatches').textContent = searchMatches.length;
        document.getElementById('cgStatRoots').textContent = rootSet.size;
        let maxDepth = 0;
        nodeDepths.forEach((depth) => {
            maxDepth = Math.max(maxDepth, Math.abs(depth));
        });
        document.getElementById('cgStatDepth').textContent = maxDepth;
        const matchIndices = new Set(searchMatches.map(n => n.index));
        let visibleCount = chainSet.size;
        searchMatches.forEach(n => {
            if (!chainSet.has(n.index)) visibleCount++;
        });
        document.getElementById('cgStatVisible').textContent = visibleCount;
    }

    function focusOnNode(targetNode) {
        if (!svg || !zoom || !targetNode) return;
        const container = document.getElementById('callgraph');
        const width = container.clientWidth;
        const height = 700;
        const scale = 1.8;
        const tx = width / 2 - targetNode.x * scale;
        const ty = height / 2 - targetNode.y * scale;
        svg.transition()
            .duration(400)
            .ease(d3.easeCubicOut)
            .call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale));
    }

    function removeBridgeEdges() {
        if (linkContainer) {
            linkContainer.selectAll('path.bridge-edge').remove();
        }
        bridgeEdges = [];
        bridgeEdgeSelection = null;
        if (simulation) {
            simulation.on('tick.bridge', null);
        }
    }

    function calculateAndAddBridgeEdges() {
        if (!linkContainer || filteredNodeIndices.size === 0) return;

        const callersMapByIndex = new Map();
        const calleesMapByIndex = new Map();

        links.forEach(link => {
            const sourceIdx = link.source.index;
            const targetIdx = link.target.index;
            if (!callersMapByIndex.has(targetIdx)) callersMapByIndex.set(targetIdx, []);
            callersMapByIndex.get(targetIdx).push(sourceIdx);
            if (!calleesMapByIndex.has(sourceIdx)) calleesMapByIndex.set(sourceIdx, []);
            calleesMapByIndex.get(sourceIdx).push(targetIdx);
        });

        const bridgeEdgesMap = new Map();
        const nonFilteredIndices = new Set();
        nodes.forEach((node, index) => {
            if (!filteredNodeIndices.has(index)) {
                nonFilteredIndices.add(index);
            }
        });

        nonFilteredIndices.forEach(sourceIdx => {
            const directCallees = calleesMapByIndex.get(sourceIdx) || [];
            directCallees.forEach(calleeIdx => {
                if (filteredNodeIndices.has(calleeIdx)) {
                    const reachableTargets = findReachableIndices(calleeIdx, calleesMapByIndex, filteredNodeIndices, nonFilteredIndices);
                    reachableTargets.forEach(targetIdx => {
                        if (targetIdx !== sourceIdx) {
                            bridgeEdgesMap.set(`${sourceIdx}->${targetIdx}`, true);
                        }
                    });
                }
            });
        });

        bridgeEdges = [];
        bridgeEdgesMap.forEach((_, key) => {
            const [sourceIdxStr, targetIdxStr] = key.split('->');
            const sourceIdx = parseInt(sourceIdxStr);
            const targetIdx = parseInt(targetIdxStr);
            bridgeEdges.push({
                source: nodes[sourceIdx],
                target: nodes[targetIdx],
                isBridge: true
            });
        });

        if (bridgeEdges.length > 0) {
            addBridgeEdgesToGraph();
        }
    }

    function findReachableIndices(startIdx, calleesMapByIndex, filteredIndices, validIndices) {
        const reachable = [];
        const visited = new Set();
        const queue = [startIdx];

        while (queue.length > 0) {
            const currentIdx = queue.shift();
            if (visited.has(currentIdx)) continue;
            visited.add(currentIdx);

            if (validIndices.has(currentIdx)) {
                reachable.push(currentIdx);
            } else if (filteredIndices.has(currentIdx)) {
                const callees = calleesMapByIndex.get(currentIdx) || [];
                callees.forEach(calleeIdx => {
                    if (!visited.has(calleeIdx)) {
                        queue.push(calleeIdx);
                    }
                });
            }
        }
        return reachable;
    }

    function addBridgeEdgesToGraph() {
        if (!linkContainer || bridgeEdges.length === 0) return;

        bridgeEdgeSelection = linkContainer
            .selectAll('path.bridge-edge')
            .data(bridgeEdges)
            .enter()
            .append('path')
            .attr('class', 'callgraph-edge bridge-edge')
            .attr('stroke-width', 1.5)
            .attr('marker-end', 'url(#arrowhead-bridge)')
            .attr('fill', 'none')
            .attr('d', d => calculateEdgePath(d));

        if (simulation) {
            simulation.on('tick.bridge', () => {
                if (bridgeEdgeSelection) {
                    bridgeEdgeSelection.attr('d', d => calculateEdgePath(d));
                }
            });
        }
    }

    // Public API
    return {
        init: function() {
            const searchInput = document.getElementById('callgraphSearchInput');
            if (searchInput) {
                searchInput.addEventListener('keyup', (e) => {
                    if (e.key === 'Enter') this.search();
                    else if (e.key === 'ArrowDown' || e.key === 'n') {
                        e.preventDefault();
                        this.navigateMatch(1);
                    } else if (e.key === 'ArrowUp' || e.key === 'p') {
                        e.preventDefault();
                        this.navigateMatch(-1);
                    } else if (e.key === 'Escape') {
                        this.clearSearch();
                    }
                });
            }
            
            // Setup click outside to close thread dropdown
            document.addEventListener('click', (e) => {
                const wrapper = document.getElementById('threadSelectorWrapper');
                if (wrapper && !wrapper.contains(e.target)) {
                    this.toggleThreadDropdown(false);
                }
            });
        },

        async load(taskId, type = '') {
            const container = document.getElementById('callgraph');
            container.innerHTML = '<div class="loading">Loading call graph</div>';

            try {
                const data = await API.getCallGraph(taskId, type);
                originalData = data;
                
                // Extract thread call graphs if available
                threadCallGraphs = [];
                hasThreadData = false;
                selectedThreadTid = null;
                
                if (data.analysis && data.analysis.threadCallGraphs && data.analysis.threadCallGraphs.length > 0) {
                    threadCallGraphs = data.analysis.threadCallGraphs;
                    hasThreadData = true;
                }
                
                // Update thread selector UI
                this.updateThreadSelector();
                
                this.render(data);
            } catch (err) {
                console.error('Failed to load call graph:', err);
                container.innerHTML = '<div class="loading">Failed to load call graph: ' + err.message + '</div>';
            }
        },
        
        // Update thread selector dropdown
        updateThreadSelector() {
            const container = document.getElementById('threadSelectorContainer');
            const countBadge = document.getElementById('threadCount');
            const dropdown = document.getElementById('threadDropdown');
            const selectedText = document.getElementById('threadSelectedText');
            
            if (!container || !dropdown) return;
            
            if (!hasThreadData || threadCallGraphs.length === 0) {
                container.style.display = 'none';
                return;
            }
            
            container.style.display = 'flex';
            
            // Update thread count badge
            if (countBadge) {
                countBadge.textContent = threadCallGraphs.length;
            }
            
            // Reset selected text
            if (selectedText) {
                selectedText.innerHTML = '<span class="global-icon">🌐</span> Global View (All Threads)';
            }
            
            // Build dropdown items
            this.renderThreadDropdownItems('');
        },
        
        // Render filtered thread dropdown items
        renderThreadDropdownItems(filter) {
            const dropdown = document.getElementById('threadDropdown');
            if (!dropdown) return;
            
            const filterLower = filter.toLowerCase();
            let html = '';
            
            // Global view option (always show unless filter doesn't match)
            if (!filter || 'global'.includes(filterLower) || 'all threads'.includes(filterLower)) {
                html += `<div class="thread-dropdown-item global-item" data-tid="" onclick="selectCallGraphThread('')">
                    <span class="item-icon">🌐</span>
                    <span class="item-text">Global View (All Threads)</span>
                </div>`;
            }
            
            // Filter and render thread items
            const filteredThreads = threadCallGraphs.filter(tcg => {
                if (!filter) return true;
                const name = (tcg.threadName || `Thread-${tcg.tid}`).toLowerCase();
                const group = (tcg.threadGroup || '').toLowerCase();
                const tid = String(tcg.tid);
                return name.includes(filterLower) || group.includes(filterLower) || tid.includes(filterLower);
            });
            
            if (filteredThreads.length > 0) {
                html += '<div class="thread-dropdown-group-label">Threads by CPU Usage</div>';
                
                filteredThreads.forEach(tcg => {
                    const pct = tcg.percentage ? tcg.percentage.toFixed(1) : '0.0';
                    const samples = tcg.totalSamples || 0;
                    const displayName = tcg.threadName || `Thread-${tcg.tid}`;
                    const groupInfo = tcg.threadGroup ? `[${tcg.threadGroup}]` : '';
                    const isSelected = selectedThreadTid === tcg.tid;
                    
                    html += `<div class="thread-dropdown-item ${isSelected ? 'selected' : ''}" data-tid="${tcg.tid}" onclick="selectCallGraphThread('${tcg.tid}')" title="TID: ${tcg.tid}, Samples: ${samples}">
                        <div class="item-main">
                            <span class="item-icon">🧵</span>
                            <span class="item-text">${this.escapeHtml(displayName)}</span>
                            ${groupInfo ? `<span class="item-group">${this.escapeHtml(groupInfo)}</span>` : ''}
                        </div>
                        <span class="item-pct">${pct}%</span>
                    </div>`;
                });
            }
            
            if (!html) {
                html = '<div class="thread-dropdown-empty">No threads match your search</div>';
            }
            
            dropdown.innerHTML = html;
        },
        
        // Escape HTML for safe rendering
        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        },
        
        // Toggle thread dropdown visibility
        toggleThreadDropdown(show) {
            const wrapper = document.getElementById('threadSelectorWrapper');
            const input = document.getElementById('threadSearchInput');
            if (!wrapper) return;
            
            if (show === undefined) {
                show = !wrapper.classList.contains('open');
            }
            
            if (show) {
                wrapper.classList.add('open');
                if (input) {
                    input.focus();
                    input.select();
                }
                // Re-render with current filter
                this.renderThreadDropdownItems(input ? input.value : '');
            } else {
                wrapper.classList.remove('open');
            }
        },
        
        // Handle thread search input
        handleThreadSearch(value) {
            this.renderThreadDropdownItems(value);
        },
        
        // Handle keyboard navigation in thread dropdown
        handleThreadSearchKeydown(event) {
            const dropdown = document.getElementById('threadDropdown');
            if (!dropdown) return;
            
            const items = dropdown.querySelectorAll('.thread-dropdown-item:not(.disabled)');
            const currentIndex = Array.from(items).findIndex(item => item.classList.contains('highlighted'));
            
            switch (event.key) {
                case 'ArrowDown':
                    event.preventDefault();
                    if (currentIndex < items.length - 1) {
                        items.forEach(item => item.classList.remove('highlighted'));
                        items[currentIndex + 1].classList.add('highlighted');
                        items[currentIndex + 1].scrollIntoView({ block: 'nearest' });
                    } else if (currentIndex === -1 && items.length > 0) {
                        items[0].classList.add('highlighted');
                    }
                    break;
                case 'ArrowUp':
                    event.preventDefault();
                    if (currentIndex > 0) {
                        items.forEach(item => item.classList.remove('highlighted'));
                        items[currentIndex - 1].classList.add('highlighted');
                        items[currentIndex - 1].scrollIntoView({ block: 'nearest' });
                    }
                    break;
                case 'Enter':
                    event.preventDefault();
                    const highlighted = dropdown.querySelector('.thread-dropdown-item.highlighted');
                    if (highlighted) {
                        highlighted.click();
                    } else if (items.length > 0) {
                        items[0].click();
                    }
                    break;
                case 'Escape':
                    event.preventDefault();
                    this.toggleThreadDropdown(false);
                    break;
            }
        },
        
        // Handle thread selection change
        selectThread(tid) {
            const selectedText = document.getElementById('threadSelectedText');
            const input = document.getElementById('threadSearchInput');
            
            if (tid === '' || tid === null || tid === undefined) {
                selectedThreadTid = null;
                // Update display
                if (selectedText) {
                    selectedText.innerHTML = '<span class="global-icon">🌐</span> Global View (All Threads)';
                }
                // Clear search input
                if (input) input.value = '';
                // Close dropdown
                this.toggleThreadDropdown(false);
                // Render global view
                if (originalData) {
                    this.clearSearch();
                    this.render(originalData);
                }
            } else {
                selectedThreadTid = parseInt(tid);
                // Find the selected thread
                const tcg = threadCallGraphs.find(t => t.tid === selectedThreadTid);
                if (tcg) {
                    // Update display
                    if (selectedText) {
                        const displayName = tcg.threadName || `Thread-${tcg.tid}`;
                        const pct = tcg.percentage ? tcg.percentage.toFixed(1) : '0.0';
                        selectedText.innerHTML = `<span class="thread-icon">🧵</span> ${this.escapeHtml(displayName)} <span class="selected-pct">(${pct}%)</span>`;
                    }
                    // Clear search input
                    if (input) input.value = '';
                    // Close dropdown
                    this.toggleThreadDropdown(false);
                    // Render thread call graph
                    this.clearSearch();
                    this.renderThreadCallGraph(tcg);
                }
            }
        },
        
        // Render a thread-specific call graph
        renderThreadCallGraph(tcg) {
            const container = document.getElementById('callgraph');
            container.innerHTML = '';
            
            if (!tcg || !tcg.nodes || tcg.nodes.length === 0) {
                container.innerHTML = '<div class="loading">No call graph data for this thread</div>';
                return;
            }
            
            // Create a data object compatible with render()
            const threadData = {
                nodes: tcg.nodes,
                edges: tcg.edges,
                totalSamples: tcg.totalSamples
            };
            
            this.render(threadData);
        },

        render(data) {
            const container = document.getElementById('callgraph');
            container.innerHTML = '';

            if (!data || !data.nodes || data.nodes.length === 0) {
                container.innerHTML = '<div class="loading">No call graph data available</div>';
                return;
            }
            
            // Store the current theme for later comparison
            this._lastRenderedTheme = document.documentElement.getAttribute('data-theme') || 'light';

            const width = container.clientWidth || 1200;
            const height = 700;

            svg = d3.select('#callgraph')
                .append('svg')
                .attr('width', width)
                .attr('height', height);

            const g = svg.append('g');

            zoom = d3.zoom()
                .scaleExtent([0.1, 4])
                .on('zoom', (event) => {
                    g.attr('transform', event.transform);
                });

            svg.call(zoom);

            const nodeMap = new Map();
            data.nodes.forEach((node, i) => {
                nodeMap.set(node.id, i);
            });

            links = data.edges.map(edge => ({
                source: nodeMap.get(edge.source),
                target: nodeMap.get(edge.target),
                weight: edge.weight || 1
            })).filter(l => l.source !== undefined && l.target !== undefined);

            nodes = data.nodes.map((node, i) => ({
                ...node,
                index: i,
                x: width / 2 + (Math.random() - 0.5) * 200,
                y: height / 2 + (Math.random() - 0.5) * 200
            }));

            simulation = d3.forceSimulation(nodes)
                .force('link', d3.forceLink(links).id(d => d.index).distance(120).strength(0.5))
                .force('charge', d3.forceManyBody().strength(-400))
                .force('center', d3.forceCenter(width / 2, height / 2))
                .force('collision', d3.forceCollide().radius(d => {
                    const w = getNodeWidth(d.name);
                    const h = getNodeHeight(d.name);
                    return Math.max(w, h) / 2 + 15;
                }));

            // Create arrow markers with theme-aware colors
            const defs = svg.append('defs');
            const arrowColors = this.getArrowColors();
            const markers = [
                { id: 'arrowhead', fill: arrowColors.default },
                { id: 'arrowhead-highlight', fill: arrowColors.highlight },
                { id: 'arrowhead-chain', fill: arrowColors.chain },
                { id: 'arrowhead-bridge', fill: arrowColors.bridge },
                { id: 'arrowhead-callee', fill: arrowColors.callee }
            ];
            markers.forEach(m => {
                defs.append('marker')
                    .attr('id', m.id)
                    .attr('viewBox', '-0 -5 10 10')
                    .attr('refX', 10)
                    .attr('refY', 0)
                    .attr('orient', 'auto')
                    .attr('markerWidth', m.id === 'arrowhead' ? 6 : 7)
                    .attr('markerHeight', m.id === 'arrowhead' ? 6 : 7)
                    .append('path')
                    .attr('d', 'M 0,-5 L 10,0 L 0,5')
                    .attr('fill', m.fill);
            });

            linkContainer = g.append('g').attr('class', 'links-container');
            const link = linkContainer
                .selectAll('path.callgraph-edge:not(.bridge-edge)')
                .data(links)
                .enter()
                .append('path')
                .attr('class', 'callgraph-edge')
                .attr('stroke-width', d => Math.max(1, Math.min(5, d.weight / 5)))
                .attr('marker-end', 'url(#arrowhead)')
                .attr('fill', 'none');

            const node = g.append('g')
                .selectAll('g')
                .data(nodes)
                .enter()
                .append('g')
                .attr('class', 'callgraph-node')
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

            // Get theme-aware colors from CSS variables
            const rootStyle = getComputedStyle(document.documentElement);
            const flameCool = rootStyle.getPropertyValue('--color-flame-cool').trim() || '255 213 79';
            const flameWarm = rootStyle.getPropertyValue('--color-flame-warm').trim() || '255 167 38';
            const flameHot = rootStyle.getPropertyValue('--color-flame-hot').trim() || '255 112 67';
            
            const colorScale = d3.scaleLinear()
                .domain([0, 10, 50])
                .range([`rgb(${flameCool})`, `rgb(${flameWarm})`, `rgb(${flameHot})`]);

            // Draw node rectangles with dynamic height
            node.append('rect')
                .attr('width', d => getNodeWidth(d.name))
                .attr('height', d => getNodeHeight(d.name))
                .attr('x', d => -getNodeWidth(d.name) / 2)
                .attr('y', d => -getNodeHeight(d.name) / 2)
                .attr('rx', 4)
                .attr('fill', d => colorScale(d.selfPct || 0));

            // Draw multi-line text
            // Line 1: package path (small, gray)
            // Line 2: class name (medium, dark)
            // Line 3: method name with arrow (bold, black)
            // Use CSS classes for theme-aware colors
            node.each(function(d) {
                const nodeGroup = d3.select(this);
                const { lines } = getMultiLineDisplayName(d.name);
                const lineHeight = NODE_CONFIG.lineHeight;
                const startY = -((lines.length - 1) * lineHeight) / 2;
                
                lines.forEach((line, i) => {
                    // Determine line type: 0=package, 1=class, 2=method (or last line)
                    const isMethodLine = (lines.length === 3 && i === 2) || (lines.length < 3 && i === lines.length - 1);
                    const isClassLine = lines.length === 3 && i === 1;
                    const isPackageLine = lines.length === 3 && i === 0;
                    
                    let fontSize = '10px';
                    let fontWeight = '400';
                    let textClass = 'node-text node-text-secondary';
                    
                    if (isMethodLine) {
                        fontSize = '10px';
                        fontWeight = '600';
                        textClass = 'node-text node-text-primary';
                    } else if (isClassLine) {
                        fontSize = '10px';
                        fontWeight = '500';
                        textClass = 'node-text node-text-base';
                    } else if (isPackageLine) {
                        fontSize = '9px';
                        fontWeight = '400';
                        textClass = 'node-text node-text-muted';
                    }
                    
                    nodeGroup.append('text')
                        .attr('class', textClass)
                        .attr('text-anchor', 'middle')
                        .attr('x', 0)
                        .attr('y', startY + i * lineHeight)
                        .attr('dy', '0.35em')
                        .text(line)
                        .style('font-size', fontSize)
                        .style('font-weight', fontWeight);
                });
            });

            const tooltip = document.getElementById('nodeTooltip');
            node.on('mouseover', function(event, d) {
                d3.select(this).classed('highlighted', true);
                link.classed('highlighted', l => l.source.index === d.index || l.target.index === d.index);
                tooltip.style.display = 'block';
                tooltip.innerHTML = buildNodeTooltip(d);
                positionTooltip(tooltip, event);
            })
            .on('mousemove', function(event) {
                positionTooltip(tooltip, event);
            })
            .on('mouseout', function() {
                d3.select(this).classed('highlighted', false);
                link.classed('highlighted', false);
                tooltip.style.display = 'none';
            });

            simulation.on('tick', () => {
                link.attr('d', d => calculateEdgePath(d));
                node.attr('transform', d => `translate(${d.x},${d.y})`);
            });

            nodeSelection = node;
            linkSelection = link;

            filteredNodeIndices = new Set();
            bridgeEdges = [];
            bridgeEdgeSelection = null;

            // Build adjacency maps for tooltip caller/callee info
            buildAdjacencyMaps();

            if (filters.size > 0) {
                setTimeout(() => {
                    this.applyFilters();
                    if (searchTerm) applySearchHighlight();
                }, 50);
            }
        },

        search() {
            const term = document.getElementById('callgraphSearchInput').value.trim();
            if (!term || nodes.length === 0) {
                this.clearSearch();
                return;
            }

            searchTerm = term;
            const termLower = term.toLowerCase();
            // Also search without allocation marker for better UX
            const termClean = Utils.stripAllocationMarker(term).toLowerCase();

            const allMatches = nodes.filter(n => {
                if (!n.name || filteredNodeIndices.has(n.index)) return false;
                const nameLower = n.name.toLowerCase();
                const nameClean = Utils.stripAllocationMarker(n.name).toLowerCase();
                return nameLower.includes(termLower) || nameClean.includes(termClean);
            });

            const wasLimited = allMatches.length > MAX_MATCHES;
            searchMatches = allMatches.slice(0, MAX_MATCHES);
            searchIndex = 0;

            buildAdjacencyMaps();
            analyzeCallChains();
            applySearchHighlight();
            updateSearchUI();
            updateStats();

            document.getElementById('callgraphStats').style.display = 'flex';

            if (searchMatches.length > 0) {
                this.updateSidebar();
                this.toggleSidebar(true);
                focusOnNode(searchMatches[0]);
            }
        },

        clearSearch() {
            document.getElementById('callgraphSearchInput').value = '';
            searchTerm = '';
            searchMatches = [];
            searchIndex = 0;
            chainSet = new Set();
            rootSet = new Set();
            chainEdges = new Set();
            nodeDepths = new Map();
            viewMode = 'all';

            document.getElementById('callgraphStats').style.display = 'none';
            this.toggleSidebar(false);

            const switchTrack = document.getElementById('viewModeSwitch');
            const leftLabel = document.querySelector('.view-mode-switch .switch-label.left');
            const rightLabel = document.querySelector('.view-mode-switch .switch-label.right');
            if (switchTrack) switchTrack.classList.remove('focus-mode');
            if (leftLabel) leftLabel.classList.add('active');
            if (rightLabel) rightLabel.classList.remove('active');

            if (nodeSelection) {
                nodeSelection
                    .classed('search-match', false)
                    .classed('current-focus', false)
                    .classed('search-chain', false)
                    .classed('search-root', false)
                    .classed('search-dimmed', false)
                    .classed('search-hidden', false);
            }
            if (linkSelection) {
                linkSelection
                    .classed('search-connected', false)
                    .classed('search-chain', false)
                    .classed('search-dimmed', false)
                    .classed('search-hidden', false)
                    .attr('marker-end', 'url(#arrowhead)');
            }

            updateSearchUI();
        },

        navigateMatch(direction) {
            if (searchMatches.length === 0) return;
            searchIndex = (searchIndex + direction + searchMatches.length) % searchMatches.length;
            analyzeCallChains();
            applySearchHighlight();
            updateSearchUI();
            this.updateSidebar();
            focusOnNode(searchMatches[searchIndex]);
        },

        toggleViewMode() {
            const newMode = viewMode === 'all' ? 'focus' : 'all';
            this.setViewMode(newMode);
        },

        setViewMode(mode) {
            viewMode = mode;
            const switchTrack = document.getElementById('viewModeSwitch');
            const leftLabel = document.querySelector('.view-mode-switch .switch-label.left');
            const rightLabel = document.querySelector('.view-mode-switch .switch-label.right');

            if (switchTrack) switchTrack.classList.toggle('focus-mode', mode === 'focus');
            if (leftLabel) leftLabel.classList.toggle('active', mode === 'all');
            if (rightLabel) rightLabel.classList.toggle('active', mode === 'focus');

            this.applyFilters();
            if (searchTerm) {
                applySearchHighlight();
                if (mode === 'focus') {
                    setTimeout(() => this.fitToVisible(), 100);
                }
            }
        },

        toggleFilter(filterType) {
            const chip = document.querySelector(`#callgraphFilterSection .filter-chip[data-filter="${filterType}"]`);
            if (filters.has(filterType)) {
                filters.delete(filterType);
                chip.classList.remove('active');
            } else {
                filters.add(filterType);
                chip.classList.add('active');
            }
            this.applyFilters();
            if (searchTerm) applySearchHighlight();
        },

        applyFilters() {
            if (!nodeSelection || !linkSelection || nodes.length === 0) return;

            removeBridgeEdges();

            if (filters.size === 0) {
                filteredNodeIndices = new Set();
                nodeSelection.classed('filter-hidden', false);
                linkSelection.classed('filter-hidden', false);
                return;
            }

            filteredNodeIndices = new Set();
            nodes.forEach((node, index) => {
                if (isSystemFunction(node.name, filters)) {
                    filteredNodeIndices.add(index);
                }
            });

            if (filteredNodeIndices.size === 0) return;

            nodeSelection.classed('filter-hidden', d => filteredNodeIndices.has(d.index));
            linkSelection.classed('filter-hidden', d =>
                filteredNodeIndices.has(d.source.index) || filteredNodeIndices.has(d.target.index)
            );

            calculateAndAddBridgeEdges();
        },

        fit() {
            if (!svg || !zoom) return;
            const container = document.getElementById('callgraph');
            const bounds = svg.select('g').node().getBBox();
            const width = container.clientWidth;
            const height = 700;
            const scale = Math.min(width / (bounds.width + 100), height / (bounds.height + 100), 1);
            const tx = (width - bounds.width * scale) / 2 - bounds.x * scale;
            const ty = (height - bounds.height * scale) / 2 - bounds.y * scale;
            svg.transition().duration(500).call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale));
        },

        fitToVisible() {
            if (!svg || !zoom || !searchTerm) return;
            const container = document.getElementById('callgraph');
            const width = container.clientWidth;
            const height = 700;
            const matchSet = new Set(searchMatches.map(n => n.index));
            const visibleNodes = new Set([...matchSet, ...chainSet]);

            let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
            nodes.forEach(node => {
                if (visibleNodes.has(node.index)) {
                    minX = Math.min(minX, node.x - 100);
                    maxX = Math.max(maxX, node.x + 100);
                    minY = Math.min(minY, node.y - 20);
                    maxY = Math.max(maxY, node.y + 20);
                }
            });

            if (minX === Infinity) return;

            const boundsWidth = maxX - minX;
            const boundsHeight = maxY - minY;
            const scale = Math.min(width / (boundsWidth + 50), height / (boundsHeight + 50), 2);
            const tx = (width - boundsWidth * scale) / 2 - minX * scale;
            const ty = (height - boundsHeight * scale) / 2 - minY * scale;

            svg.transition()
                .duration(500)
                .ease(d3.easeCubicOut)
                .call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale));
        },

        reset() {
            if (simulation) simulation.alpha(1).restart();
            if (svg && zoom) {
                svg.transition().duration(500).call(zoom.transform, d3.zoomIdentity);
            }
        },

        toggleSidebar(show) {
            const sidebar = document.getElementById('callchainSidebar');
            if (show) sidebar.classList.add('active');
            else sidebar.classList.remove('active');
        },

        updateSidebar() {
            const content = document.getElementById('callchainContent');
            const currentMatch = searchMatches[searchIndex];

            if (!currentMatch) {
                content.innerHTML = '<div style="padding: 15px; color: #888;">No matches found</div>';
                return;
            }

            const chainNodes = [];
            const visited = new Set();
            const queue = [[currentMatch.index, 0]];
            visited.add(currentMatch.index);

            const MAX_SIDEBAR_NODES = 100;
            const MAX_SIDEBAR_DEPTH = 30;

            while (queue.length > 0 && chainNodes.length < MAX_SIDEBAR_NODES) {
                const [nodeIndex, depth] = queue.shift();
                if (Math.abs(depth) > MAX_SIDEBAR_DEPTH) continue;

                const node = nodes[nodeIndex];
                if (!node) continue;

                const isFiltered = filteredNodeIndices.has(nodeIndex);
                if (!isFiltered) {
                    const isMatch = searchMatches.some(m => m.index === nodeIndex);
                    const isRoot = rootSet.has(nodeIndex);
                    chainNodes.push({
                        node,
                        depth,
                        isMatch,
                        isRoot,
                        isChain: !isMatch && !isRoot
                    });
                }

                const callers = callersMap.get(nodeIndex) || [];
                callers.forEach(callerIdx => {
                    if (!visited.has(callerIdx)) {
                        visited.add(callerIdx);
                        queue.push([callerIdx, depth - 1]);
                    }
                });
            }

            chainNodes.sort((a, b) => a.depth - b.depth);

            let html = '';

            // Roots section
            const roots = chainNodes.filter(n => n.isRoot);
            if (roots.length > 0) {
                html += `<div class="callchain-section"><div class="callchain-section-title">📍 Entry Points (${roots.length})</div>`;
                const displayRoots = roots.length <= 5 ? roots : roots.slice(0, 3);
                displayRoots.forEach(item => {
                    html += `<div class="callchain-node root" onclick="CallGraph.focusNode(${item.node.index})" title="${Utils.escapeHtml(item.node.name)}">${Utils.escapeHtml(getShortName(item.node.name))}</div>`;
                });
                if (roots.length > 5) {
                    html += `<div class="callchain-node" style="color: #888; font-style: italic;">... and ${roots.length - 3} more</div>`;
                }
                if (chainNodes.length > roots.length) html += `<div class="callchain-arrow">↓</div>`;
                html += `</div>`;
            }

            // Chain section
            const chain = chainNodes.filter(n => n.isChain);
            if (chain.length > 0) {
                html += `<div class="callchain-section"><div class="callchain-section-title">🔗 Call Chain (${chain.length})</div>`;
                if (chain.length <= 10) {
                    chain.forEach((item, idx) => {
                        html += `<div class="callchain-node chain" onclick="CallGraph.focusNode(${item.node.index})" title="${Utils.escapeHtml(item.node.name)}">${Utils.escapeHtml(getShortName(item.node.name))}</div>`;
                        if (idx < chain.length - 1) html += `<div class="callchain-arrow">↓</div>`;
                    });
                } else {
                    chain.slice(0, 3).forEach(item => {
                        html += `<div class="callchain-node chain" onclick="CallGraph.focusNode(${item.node.index})">${Utils.escapeHtml(getShortName(item.node.name))}</div><div class="callchain-arrow">↓</div>`;
                    });
                    html += `<div class="callchain-node" style="color: #888; text-align: center;">⋮ ${chain.length - 6} more ⋮</div><div class="callchain-arrow">↓</div>`;
                    chain.slice(-3).forEach((item, idx) => {
                        html += `<div class="callchain-node chain" onclick="CallGraph.focusNode(${item.node.index})">${Utils.escapeHtml(getShortName(item.node.name))}</div>`;
                        if (idx < 2) html += `<div class="callchain-arrow">↓</div>`;
                    });
                }
                html += `<div class="callchain-arrow">↓</div></div>`;
            }

            // Match section
            const matches = chainNodes.filter(n => n.isMatch);
            if (matches.length > 0) {
                html += `<div class="callchain-section"><div class="callchain-section-title">🎯 Target Match</div>`;
                matches.forEach(item => {
                    html += `<div class="callchain-node match" onclick="CallGraph.focusNode(${item.node.index})">${Utils.escapeHtml(getShortName(item.node.name))}</div>`;
                });
                html += `</div>`;
            }

            // Callees section - show next level calls with percentage
            const calleesData = this.collectCallees(currentMatch);
            if (calleesData.length > 0) {
                html += `<div class="callchain-section">`;
                html += `<div class="callchain-section-title">📤 Callees (${calleesData.length})</div>`;
                
                const MAX_CALLEES_DISPLAY = 8;
                const displayCallees = calleesData.slice(0, MAX_CALLEES_DISPLAY);
                const othersCallees = calleesData.slice(MAX_CALLEES_DISPLAY);
                
                displayCallees.forEach(item => {
                    const pctWidth = Math.min(100, Math.max(5, item.weight));
                    const pctText = item.weight.toFixed(1);
                    const timeText = item.selfTime ? Utils.formatTime(item.selfTime) : '';
                    const isFiltered = filteredNodeIndices.has(item.node.index);
                    const nodeClass = isFiltered ? 'callee filtered' : 'callee';
                    
                    html += `<div class="callchain-node ${nodeClass}" onclick="CallGraph.focusNode(${item.node.index})" title="${Utils.escapeHtml(item.node.name)}">
                        <div class="callee-progress-bar">
                            <div class="callee-progress" style="width: ${pctWidth}%"></div>
                        </div>
                        <div class="callee-info">
                            <span class="callee-name">${Utils.escapeHtml(getShortName(item.node.name))}</span>
                            <span class="callee-stats">${pctText}%${timeText ? ' · ' + timeText : ''}</span>
                        </div>
                    </div>`;
                });
                
                // Show collapsed "others" if there are more callees
                if (othersCallees.length > 0) {
                    const othersWeight = othersCallees.reduce((sum, c) => sum + c.weight, 0);
                    html += `<div class="callchain-node callee-others" onclick="CallGraph.toggleCalleesExpand()">
                        <span class="others-label">+ ${othersCallees.length} more</span>
                        <span class="others-pct">${othersWeight.toFixed(1)}%</span>
                    </div>`;
                }
                
                html += `</div>`;
            }

            content.innerHTML = html;
        },

        focusNode(index) {
            const node = nodes[index];
            if (node) focusOnNode(node);
        },

        // Collect callees (next level calls) for a given node with weight info
        collectCallees(matchNode) {
            if (!matchNode || !calleesMap) return [];
            
            const calleeIndices = calleesMap.get(matchNode.index) || [];
            if (calleeIndices.length === 0) return [];
            
            const calleesWithWeight = calleeIndices.map(calleeIdx => {
                const calleeNode = nodes[calleeIdx];
                if (!calleeNode) return null;
                
                // Find the edge to get weight
                const edge = links.find(l => 
                    l.source.index === matchNode.index && 
                    l.target.index === calleeIdx
                );
                
                return {
                    node: calleeNode,
                    weight: edge?.weight || calleeNode.totalPct || 0,
                    selfTime: calleeNode.selfTime || 0,
                    totalTime: calleeNode.totalTime || 0
                };
            }).filter(item => item !== null);
            
            // Sort by weight descending
            calleesWithWeight.sort((a, b) => b.weight - a.weight);
            
            return calleesWithWeight;
        },
        
        // Toggle expanded view of callees (for future enhancement)
        toggleCalleesExpand() {
            // For now, just show a message - can be enhanced to expand inline
            console.log('Expand callees - feature can be enhanced');
        },

        searchFor(funcName) {
            const term = Utils.extractSearchTerm(funcName);
            document.getElementById('callgraphSearchInput').value = term;
            setTimeout(() => this.search(), 100);
        },

        getNodes() {
            return nodes;
        },
        
        // Get original data for re-rendering
        getOriginalData() {
            return originalData;
        },
        
        // Get theme-aware arrow colors from CSS variables
        getArrowColors() {
            const rootStyle = getComputedStyle(document.documentElement);
            const textMuted = rootStyle.getPropertyValue('--color-text-muted').trim() || '156 163 175';
            const flameMatch = rootStyle.getPropertyValue('--color-flame-match').trim() || '224 64 251';
            const primary = rootStyle.getPropertyValue('--color-primary').trim() || '102 126 234';
            const secondary = rootStyle.getPropertyValue('--color-secondary').trim() || '118 75 162';
            const warning = rootStyle.getPropertyValue('--color-warning').trim() || '255 152 0';
            
            return {
                default: `rgb(${textMuted})`,
                highlight: `rgb(${flameMatch})`,
                chain: `rgb(${primary})`,
                bridge: `rgb(${secondary})`,
                callee: `rgb(${warning})`
            };
        },
        
        // Get thread info for external access
        getThreadInfo() {
            return {
                hasThreadData,
                threadCount: threadCallGraphs.length,
                selectedThreadTid,
                threads: threadCallGraphs.map(t => ({
                    tid: t.tid,
                    name: t.threadName,
                    group: t.threadGroup,
                    percentage: t.percentage
                }))
            };
        },
        
        // Check and re-render if theme has changed since last render
        checkThemeAndRerender() {
            const currentTheme = document.documentElement.getAttribute('data-theme') || 'light';
            if (this._lastRenderedTheme && this._lastRenderedTheme !== currentTheme) {
                console.log('[CallGraph] Theme changed from', this._lastRenderedTheme, 'to', currentTheme, '- re-rendering');
                const data = this.getOriginalData();
                if (data && nodes.length > 0) {
                    this.render(data);
                }
            }
        },
        
        // Store the theme used for rendering
        _lastRenderedTheme: null
    };
})();

// Export for global access
window.CallGraph = CallGraph;

// Listen for theme changes to re-render call graph with new colors
if (typeof ThemeManager !== 'undefined') {
    ThemeManager.onChange(function(themeId) {
        // Re-render call graph when theme changes
        // Check if call graph is visible and has data
        const container = document.getElementById('callgraph');
        const panel = container?.closest('[x-show]') || document.getElementById('callgraph-panel');
        const isVisible = panel && (
            window.getComputedStyle(panel).display !== 'none' ||
            panel.classList.contains('active')
        );
        
        if (isVisible && CallGraph.getNodes()?.length > 0) {
            console.log('[CallGraph] Theme changed to:', themeId, '- re-rendering');
            // Re-render using the stored original data
            setTimeout(() => {
                const data = CallGraph.getOriginalData();
                if (data) {
                    CallGraph.render(data);
                }
            }, 100);
        }
        // Note: If not visible, checkThemeAndRerender() will be called when panel becomes visible
    });
}
