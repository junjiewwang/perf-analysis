/**
 * PerfScope — CPU Call Tree View
 * Bottom-up view: shows functions sorted by self time, expanding into callers.
 */
ViewRouter.register('cpu', {
  id: 'calltree',
  label: 'Call Tree',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 13h12M3 9h8M5 5h4"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter functions..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <span class="legend-item" style="font-size:11px;color:var(--color-text-secondary);">Bottom-Up · Sorted by Self % ↓</span>
        </div>
      </div>
      <div class="tree-view">
        <div class="tree-node depth-0 expanded">
          <div class="tree-row">
            <span class="tree-expand">▼</span>
            <span class="tree-name"><code>json.Marshal</code></span>
            <span class="tree-self very-hot">28.0%</span>
            <span class="tree-pct">28.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:28%"></div></div></span>
          </div>
          <div class="tree-children">
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-expand">▶</span>
                <span class="tree-name dim">← called by <code>service.ProcessOrder</code></span>
                <span class="tree-pct">72%</span>
                <span class="tree-self"></span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:72%"></div></div></span>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-expand">▶</span>
                <span class="tree-name dim">← called by <code>handler.GetUser</code></span>
                <span class="tree-pct">18%</span>
                <span class="tree-self"></span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:18%"></div></div></span>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-expand">▶</span>
                <span class="tree-name dim">← called by <code>cache.Serialize</code></span>
                <span class="tree-pct">10%</span>
                <span class="tree-self"></span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:10%"></div></div></span>
              </div>
            </div>
          </div>
        </div>
        <div class="tree-node depth-0 expanded">
          <div class="tree-row">
            <span class="tree-expand">▼</span>
            <span class="tree-name"><code>runtime.scanobject</code></span>
            <span class="tree-self hot">18.2%</span>
            <span class="tree-pct warn">23.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:23%"></div></div></span>
          </div>
          <div class="tree-children">
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-expand">▶</span>
                <span class="tree-name dim">← called by <code>runtime.gcBgMarkWorker</code></span>
                <span class="tree-pct">100%</span>
                <span class="tree-self"></span>
                <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:100%"></div></div></span>
              </div>
            </div>
          </div>
        </div>
        <div class="tree-node depth-0">
          <div class="tree-row">
            <span class="tree-expand">▶</span>
            <span class="tree-name"><code>syscall.Read</code></span>
            <span class="tree-self">15.0%</span>
            <span class="tree-pct">15.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:15%"></div></div></span>
          </div>
        </div>
        <div class="tree-node depth-0">
          <div class="tree-row">
            <span class="tree-expand">▶</span>
            <span class="tree-name"><code>crypto/tls.Verify</code></span>
            <span class="tree-self">14.0%</span>
            <span class="tree-pct">14.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:14%"></div></div></span>
          </div>
        </div>
        <div class="tree-node depth-0">
          <div class="tree-row">
            <span class="tree-expand">▶</span>
            <span class="tree-name"><code>db.Query</code></span>
            <span class="tree-self">10.0%</span>
            <span class="tree-pct">10.0%</span>
            <span class="tree-bar"><div class="bar-bg"><div class="bar-fill-heat" style="width:10%"></div></div></span>
          </div>
        </div>
      </div>`;
  }
});
