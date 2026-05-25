/**
 * PerfScope — Heap Dominator Tree View
 * Displays the dominator tree showing object retention hierarchy.
 * Each node dominates (retains) all objects in its subtree.
 */
ViewRouter.register('heap', {
  id: 'dominator',
  label: 'Dominator Tree',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="7" cy="3" r="2"/><circle cx="4" cy="10" r="2"/><circle cx="10" cy="10" r="2"/><path d="M7 5v2M5.5 8.5 7 7M8.5 8.5 7 7"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter: class or variable name..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active">By Retained Size</button>
          <button class="viz-btn">By Depth</button>
          <div class="viz-separator"></div>
          <span class="viz-stat">Depth limit: <strong>5</strong></span>
        </div>
      </div>
      <div class="tree-view dominator-tree">
        <div class="tree-node expanded depth-0">
          <div class="tree-row">
            <span class="tree-toggle">▼</span>
            <span class="tree-icon obj">◆</span>
            <span class="tree-label"><code>GC Root</code></span>
            <span class="tree-size">512.3 MB</span>
            <span class="tree-pct">100%</span>
            <span class="tree-bar"><div class="bar-fill" style="width:100%"></div></span>
          </div>
          <div class="tree-children">
            <div class="tree-node expanded depth-1">
              <div class="tree-row">
                <span class="tree-toggle">▼</span>
                <span class="tree-icon thread">⊙</span>
                <span class="tree-label"><code>main (thread)</code></span>
                <span class="tree-size hot">245.8 MB</span>
                <span class="tree-pct">48.0%</span>
                <span class="tree-bar"><div class="bar-fill hot" style="width:48%"></div></span>
              </div>
              <div class="tree-children">
                <div class="tree-node expanded depth-2">
                  <div class="tree-row">
                    <span class="tree-toggle">▼</span>
                    <span class="tree-icon obj">◆</span>
                    <span class="tree-label"><code>com.app.OrderService</code> <span class="field-name">.orderCache</span></span>
                    <span class="tree-size warn">142.3 MB</span>
                    <span class="tree-pct">27.8%</span>
                    <span class="tree-bar"><div class="bar-fill warn" style="width:28%"></div></span>
                  </div>
                  <div class="tree-children">
                    <div class="tree-node depth-3">
                      <div class="tree-row">
                        <span class="tree-toggle">▶</span>
                        <span class="tree-icon arr">▦</span>
                        <span class="tree-label"><code>java.util.HashMap</code> <span class="field-name">.table</span></span>
                        <span class="tree-size">98.7 MB</span>
                        <span class="tree-pct">19.3%</span>
                        <span class="tree-bar"><div class="bar-fill" style="width:19%"></div></span>
                      </div>
                    </div>
                    <div class="tree-node depth-3">
                      <div class="tree-row">
                        <span class="tree-toggle">▶</span>
                        <span class="tree-icon obj">◆</span>
                        <span class="tree-label"><code>java.util.ArrayList</code> <span class="field-name">.pendingOrders</span></span>
                        <span class="tree-size">43.6 MB</span>
                        <span class="tree-pct">8.5%</span>
                        <span class="tree-bar"><div class="bar-fill" style="width:9%"></div></span>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="tree-node depth-2">
                  <div class="tree-row">
                    <span class="tree-toggle">▶</span>
                    <span class="tree-icon obj">◆</span>
                    <span class="tree-label"><code>com.app.CacheManager</code> <span class="field-name">.entries</span></span>
                    <span class="tree-size">78.2 MB</span>
                    <span class="tree-pct">15.3%</span>
                    <span class="tree-bar"><div class="bar-fill" style="width:15%"></div></span>
                  </div>
                </div>
                <div class="tree-node depth-2">
                  <div class="tree-row">
                    <span class="tree-toggle">▶</span>
                    <span class="tree-icon obj">◆</span>
                    <span class="tree-label"><code>io.netty.buffer.PoolArena</code></span>
                    <span class="tree-size">25.3 MB</span>
                    <span class="tree-pct">4.9%</span>
                    <span class="tree-bar"><div class="bar-fill" style="width:5%"></div></span>
                  </div>
                </div>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-toggle">▶</span>
                <span class="tree-icon thread">⊙</span>
                <span class="tree-label"><code>http-worker-1 (thread)</code></span>
                <span class="tree-size">89.4 MB</span>
                <span class="tree-pct">17.5%</span>
                <span class="tree-bar"><div class="bar-fill" style="width:17%"></div></span>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-toggle">▶</span>
                <span class="tree-icon thread">⊙</span>
                <span class="tree-label"><code>scheduler-pool-3 (thread)</code></span>
                <span class="tree-size">67.1 MB</span>
                <span class="tree-pct">13.1%</span>
                <span class="tree-bar"><div class="bar-fill" style="width:13%"></div></span>
              </div>
            </div>
            <div class="tree-node depth-1">
              <div class="tree-row">
                <span class="tree-toggle">▶</span>
                <span class="tree-icon static">▣</span>
                <span class="tree-label"><code>Static Fields</code></span>
                <span class="tree-size">110.0 MB</span>
                <span class="tree-pct">21.5%</span>
                <span class="tree-bar"><div class="bar-fill" style="width:21%"></div></span>
              </div>
            </div>
          </div>
        </div>
      </div>`;
  }
});
