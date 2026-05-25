/**
 * PerfScope — Heap Merged Paths View
 * Displays merged reference paths showing how objects accumulate in memory.
 * Paths are grouped by retainer class, revealing common retention patterns.
 */
ViewRouter.register('heap', {
  id: 'mergedpaths',
  label: 'Merged Paths',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 2h3v3H2zM9 2h3v3H9zM5.5 9h3v3h-3z"/><path d="M3.5 5v2l3.5 2M10.5 5v2L7 9"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter: target class..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active">By Retained Size</button>
          <button class="viz-btn">By Count</button>
          <div class="viz-separator"></div>
          <span class="viz-stat">Patterns: <strong>18</strong></span>
        </div>
      </div>
      <div class="merged-paths-list">
        <div class="merged-path-card expanded">
          <div class="path-card-header">
            <span class="expand-icon">▼</span>
            <div class="path-card-title">
              <code>com.app.model.OrderItem</code>
              <span class="tag leak-suspect">⚠ Leak Suspect</span>
            </div>
            <div class="path-card-stats">
              <span class="stat-pill hot">234,567 instances</span>
              <span class="stat-pill">38.2 MB retained</span>
            </div>
          </div>
          <div class="path-card-body">
            <div class="retention-chain">
              <div class="chain-header">Top Retainer Paths (by accumulated retained size):</div>
              <div class="chain-item primary">
                <div class="chain-flow">
                  <span class="chain-node root">Thread:main</span>
                  <span class="chain-arrow">→</span>
                  <span class="chain-node">OrderService</span>
                  <span class="chain-arrow">.orderCache →</span>
                  <span class="chain-node">HashMap</span>
                  <span class="chain-arrow">.table[] →</span>
                  <span class="chain-node target">OrderItem</span>
                </div>
                <div class="chain-stats">
                  <span class="chain-count">189,234 instances (80.7%)</span>
                  <span class="chain-size hot">31.2 MB</span>
                </div>
                <div class="chain-bar"><div class="bar-fill hot" style="width:81%"></div></div>
              </div>
              <div class="chain-item">
                <div class="chain-flow">
                  <span class="chain-node root">Static:AppContext</span>
                  <span class="chain-arrow">→</span>
                  <span class="chain-node">ArrayList</span>
                  <span class="chain-arrow">.elementData[] →</span>
                  <span class="chain-node target">OrderItem</span>
                </div>
                <div class="chain-stats">
                  <span class="chain-count">34,521 instances (14.7%)</span>
                  <span class="chain-size">5.1 MB</span>
                </div>
                <div class="chain-bar"><div class="bar-fill" style="width:15%"></div></div>
              </div>
              <div class="chain-item">
                <div class="chain-flow">
                  <span class="chain-node root">Thread:worker-1</span>
                  <span class="chain-arrow">→</span>
                  <span class="chain-node">Queue</span>
                  <span class="chain-arrow">.items[] →</span>
                  <span class="chain-node target">OrderItem</span>
                </div>
                <div class="chain-stats">
                  <span class="chain-count">10,812 instances (4.6%)</span>
                  <span class="chain-size">1.9 MB</span>
                </div>
                <div class="chain-bar"><div class="bar-fill" style="width:5%"></div></div>
              </div>
            </div>
          </div>
        </div>
        <div class="merged-path-card">
          <div class="path-card-header">
            <span class="expand-icon">▶</span>
            <div class="path-card-title">
              <code>com.app.cache.CacheEntry</code>
              <span class="tag leak-suspect">⚠ Leak Suspect</span>
            </div>
            <div class="path-card-stats">
              <span class="stat-pill warn">98,234 instances</span>
              <span class="stat-pill">28.9 MB retained</span>
            </div>
          </div>
        </div>
        <div class="merged-path-card">
          <div class="path-card-header">
            <span class="expand-icon">▶</span>
            <div class="path-card-title">
              <code>java.util.HashMap$Node</code>
            </div>
            <div class="path-card-stats">
              <span class="stat-pill">412,897 instances</span>
              <span class="stat-pill">78.4 MB retained</span>
            </div>
          </div>
        </div>
        <div class="merged-path-card">
          <div class="path-card-header">
            <span class="expand-icon">▶</span>
            <div class="path-card-title">
              <code>io.netty.buffer.PooledByteBuf</code>
            </div>
            <div class="path-card-stats">
              <span class="stat-pill">45,678 instances</span>
              <span class="stat-pill">22.1 MB retained</span>
            </div>
          </div>
        </div>
      </div>`;
  }
});
