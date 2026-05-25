/**
 * PerfScope — CPU Treemap View
 * Visualizes CPU time as nested rectangles (area = percentage of total CPU).
 */
ViewRouter.register('cpu', {
  id: 'treemap',
  label: 'Treemap',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1" y="1" width="5" height="5"/><rect x="8" y="1" width="5" height="5"/><rect x="1" y="8" width="5" height="5"/><rect x="8" y="8" width="5" height="5"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Highlight package..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active">By Self Time</button>
          <button class="viz-btn">By Total Time</button>
          <div class="viz-separator"></div>
          <button class="viz-btn">Zoom Out</button>
        </div>
      </div>
      <div class="treemap-container">
        <div class="treemap-grid">
          <div class="treemap-cell very-hot" style="grid-column: span 3; grid-row: span 3;">
            <div class="treemap-cell-label">json.Marshal</div>
            <div class="treemap-cell-value">28.0%</div>
          </div>
          <div class="treemap-cell hot" style="grid-column: span 2; grid-row: span 2;">
            <div class="treemap-cell-label">runtime.scanobject</div>
            <div class="treemap-cell-value">18.2%</div>
          </div>
          <div class="treemap-cell" style="grid-column: span 2; grid-row: span 2;">
            <div class="treemap-cell-label">syscall.Read</div>
            <div class="treemap-cell-value">15.0%</div>
          </div>
          <div class="treemap-cell" style="grid-column: span 2; grid-row: span 2;">
            <div class="treemap-cell-label">crypto/tls.Verify</div>
            <div class="treemap-cell-value">14.0%</div>
          </div>
          <div class="treemap-cell" style="grid-column: span 1; grid-row: span 2;">
            <div class="treemap-cell-label">db.Query</div>
            <div class="treemap-cell-value">10.0%</div>
          </div>
          <div class="treemap-cell" style="grid-column: span 1; grid-row: span 2;">
            <div class="treemap-cell-label">jwt.Parse</div>
            <div class="treemap-cell-value">10.0%</div>
          </div>
          <div class="treemap-cell dim" style="grid-column: span 1; grid-row: span 1;">
            <div class="treemap-cell-label">fmt.Sprintf</div>
            <div class="treemap-cell-value">3.2%</div>
          </div>
          <div class="treemap-cell dim" style="grid-column: span 1; grid-row: span 1;">
            <div class="treemap-cell-label">sync.Pool</div>
            <div class="treemap-cell-value">1.6%</div>
          </div>
        </div>
      </div>`;
  }
});
