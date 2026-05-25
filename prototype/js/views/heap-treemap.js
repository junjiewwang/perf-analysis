/**
 * PerfScope — Heap Treemap View
 * Displays heap memory as nested rectangles proportional to retained size.
 * Color intensity indicates retained-to-shallow ratio (potential accumulation).
 */
ViewRouter.register('heap', {
  id: 'treemap',
  label: 'Treemap',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1" y="1" width="6" height="8" rx="0.5"/><rect x="8" y="1" width="5" height="4" rx="0.5"/><rect x="8" y="6" width="5" height="3" rx="0.5"/><rect x="1" y="10" width="12" height="3" rx="0.5"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="treemap-breadcrumb">
          <button class="viz-btn breadcrumb-root active">All (512.3 MB)</button>
          <span class="breadcrumb-sep">›</span>
          <span class="breadcrumb-current">Top Level View</span>
        </div>
        <div class="viz-controls">
          <button class="viz-btn" title="Color by package">By Package</button>
          <button class="viz-btn active" title="Color by heat">By Heat</button>
          <div class="viz-separator"></div>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#ef4444"></span>&gt; 50 MB</span>
            <span class="legend-item"><span class="legend-dot" style="background:#f59e0b"></span>10–50 MB</span>
            <span class="legend-item"><span class="legend-dot" style="background:#10b981"></span>1–10 MB</span>
            <span class="legend-item"><span class="legend-dot" style="background:#64748b"></span>&lt; 1 MB</span>
          </div>
        </div>
      </div>
      <div class="treemap-grid">
        <div class="treemap-cell very-hot" style="grid-column: span 4; grid-row: span 3;" data-class="byte[]">
          <div class="cell-header">byte[]</div>
          <div class="cell-value">198.7 MB</div>
          <div class="cell-count">845K instances</div>
        </div>
        <div class="treemap-cell hot" style="grid-column: span 3; grid-row: span 2;" data-class="java.lang.String">
          <div class="cell-header">String</div>
          <div class="cell-value">142.1 MB</div>
          <div class="cell-count">623K</div>
        </div>
        <div class="treemap-cell warn" style="grid-column: span 2; grid-row: span 2;" data-class="HashMap$Node">
          <div class="cell-header">HashMap$Node</div>
          <div class="cell-value">78.4 MB</div>
          <div class="cell-count">413K</div>
        </div>
        <div class="treemap-cell" style="grid-column: span 2; grid-row: span 1;" data-class="char[]">
          <div class="cell-header">char[]</div>
          <div class="cell-value">45.1 MB</div>
        </div>
        <div class="treemap-cell warn leak" style="grid-column: span 2; grid-row: span 2;" data-class="OrderItem">
          <div class="cell-header">⚠ OrderItem</div>
          <div class="cell-value">38.2 MB</div>
          <div class="cell-count">235K</div>
          <div class="cell-badge">Leak?</div>
        </div>
        <div class="treemap-cell" style="grid-column: span 2; grid-row: span 1;" data-class="ArrayList">
          <div class="cell-header">ArrayList</div>
          <div class="cell-value">34.8 MB</div>
        </div>
        <div class="treemap-cell" style="grid-column: span 1; grid-row: span 1;" data-class="Object[]">
          <div class="cell-header">Object[]</div>
          <div class="cell-value">31.5 MB</div>
        </div>
        <div class="treemap-cell warn leak" style="grid-column: span 2; grid-row: span 1;" data-class="CacheEntry">
          <div class="cell-header">⚠ CacheEntry</div>
          <div class="cell-value">28.9 MB</div>
          <div class="cell-badge">Leak?</div>
        </div>
        <div class="treemap-cell dim" style="grid-column: span 3; grid-row: span 1;" data-class="others">
          <div class="cell-header">Others (1,239 classes)</div>
          <div class="cell-value">23.4 MB</div>
        </div>
      </div>
      <div class="treemap-tooltip hidden">
        <div class="tooltip-title">byte[]</div>
        <div class="tooltip-row"><span>Retained:</span><strong>198.7 MB (38.8%)</strong></div>
        <div class="tooltip-row"><span>Shallow:</span><strong>128.4 MB</strong></div>
        <div class="tooltip-row"><span>Instances:</span><strong>845,231</strong></div>
        <div class="tooltip-hint">Click to drill down</div>
      </div>`;
  }
});
