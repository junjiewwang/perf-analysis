/**
 * PerfScope — CPU Top Down View
 * Shows functions sorted by total (inclusive) time, expanding into callees.
 */
ViewRouter.register('cpu', {
  id: 'topdown',
  label: 'Top Down',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="7" cy="7" r="5"/><path d="M7 4v3l2 2"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter functions..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active">Total Time</button>
          <button class="viz-btn">Self Time</button>
          <div class="viz-separator"></div>
          <span class="legend-item" style="font-size:11px;color:var(--color-text-secondary);">Sorted by Total % ↓</span>
        </div>
      </div>
      <div class="tree-view">
        <div class="tree-node depth-0 expanded">
          <div class="tree-row">
            <span class="tree-expand">▼</span>
            <span class="tree-name"><code>main.handleRequest</code></span>
            <span class="tree-pct hot">62.0%</span>
            <span class="tree-self">4.2%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:62%"></div></div></span>
          </div>
          <div class="tree-children">
            <div class="tree-node depth-1 expanded">
              <div class="tree-row">
                <span class="tree-expand">▼</span>
                <span class="tree-name"><code>service.ProcessOrder</code></span>
                <span class="tree-pct hot">38.0%</span>
                <span class="tree-self">2.1%</span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:38%"></div></div></span>
              </div>
              <div class="tree-children">
                <div class="tree-node depth-2">
                  <div class="tree-row">
                    <span class="tree-expand">▶</span>
                    <span class="tree-name"><code>json.Marshal</code></span>
                    <span class="tree-pct very-hot">28.0%</span>
                    <span class="tree-self hot">28.0%</span>
                    <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:28%"></div></div></span>
                  </div>
                </div>
                <div class="tree-node depth-2">
                  <div class="tree-row">
                    <span class="tree-expand">▶</span>
                    <span class="tree-name"><code>db.Query</code></span>
                    <span class="tree-pct">10.0%</span>
                    <span class="tree-self">10.0%</span>
                    <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:10%"></div></div></span>
                  </div>
                </div>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-expand">▶</span>
                <span class="tree-name"><code>middleware.Auth</code></span>
                <span class="tree-pct">24.0%</span>
                <span class="tree-self">0.5%</span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:24%"></div></div></span>
              </div>
            </div>
          </div>
        </div>
        <div class="tree-node depth-0">
          <div class="tree-row">
            <span class="tree-expand">▶</span>
            <span class="tree-name"><code>runtime.gcBgMarkWorker</code></span>
            <span class="tree-pct warn">23.0%</span>
            <span class="tree-self">5.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:23%"></div></div></span>
          </div>
        </div>
        <div class="tree-node depth-0">
          <div class="tree-row">
            <span class="tree-expand">▶</span>
            <span class="tree-name"><code>syscall.Read</code></span>
            <span class="tree-pct">15.0%</span>
            <span class="tree-self">15.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:15%"></div></div></span>
          </div>
        </div>
      </div>`;
  }
});
