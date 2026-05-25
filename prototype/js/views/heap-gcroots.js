/**
 * PerfScope — Heap GC Roots View
 * Shows the shortest paths from GC Roots to objects, grouped by root type.
 * Helps identify why objects cannot be garbage collected.
 */
ViewRouter.register('heap', {
  id: 'gcroots',
  label: 'GC Roots',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M7 1v4m0 0L4 7m3-2l3 2"/><circle cx="4" cy="9" r="2"/><circle cx="10" cy="9" r="2"/><circle cx="7" cy="13" r="1"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter: class name to find roots for..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <span class="viz-stat">Root Types: <strong>5</strong></span>
          <span class="viz-stat">Total Roots: <strong>2,847</strong></span>
        </div>
      </div>
      <div class="gcroots-summary">
        <div class="root-type-card">
          <div class="root-type-icon thread">⊙</div>
          <div class="root-type-info">
            <div class="root-type-name">Thread References</div>
            <div class="root-type-count">1,234 roots</div>
          </div>
          <div class="root-type-size hot">312.4 MB</div>
        </div>
        <div class="root-type-card">
          <div class="root-type-icon static">▣</div>
          <div class="root-type-info">
            <div class="root-type-name">Static Fields</div>
            <div class="root-type-count">892 roots</div>
          </div>
          <div class="root-type-size warn">145.2 MB</div>
        </div>
        <div class="root-type-card">
          <div class="root-type-icon jni">◈</div>
          <div class="root-type-info">
            <div class="root-type-name">JNI Global</div>
            <div class="root-type-count">423 roots</div>
          </div>
          <div class="root-type-size">32.8 MB</div>
        </div>
        <div class="root-type-card">
          <div class="root-type-icon monitor">⊡</div>
          <div class="root-type-info">
            <div class="root-type-name">Monitor (Synchronization)</div>
            <div class="root-type-count">198 roots</div>
          </div>
          <div class="root-type-size">18.4 MB</div>
        </div>
        <div class="root-type-card">
          <div class="root-type-icon finalizer">⊘</div>
          <div class="root-type-info">
            <div class="root-type-name">Finalizer</div>
            <div class="root-type-count">100 roots</div>
          </div>
          <div class="root-type-size">3.5 MB</div>
        </div>
      </div>
      <div class="gcroots-detail">
        <h4 class="section-title">Paths to GC Roots — <code>com.app.model.OrderItem</code></h4>
        <div class="tree-view gcroot-paths">
          <div class="tree-node expanded depth-0">
            <div class="tree-row root-path">
              <span class="tree-toggle">▼</span>
              <span class="tree-icon thread">⊙</span>
              <span class="tree-label"><code>Thread: main</code> <span class="path-badge">shortest path: 4</span></span>
            </div>
            <div class="tree-children">
              <div class="path-chain">
                <div class="path-step">
                  <span class="path-connector">│</span>
                  <span class="path-arrow">→</span>
                  <span class="tree-icon obj">◆</span>
                  <code>com.app.OrderService</code>
                  <span class="field-name">.orderCache</span>
                </div>
                <div class="path-step">
                  <span class="path-connector">│</span>
                  <span class="path-arrow">→</span>
                  <span class="tree-icon arr">▦</span>
                  <code>java.util.HashMap</code>
                  <span class="field-name">.table[127]</span>
                </div>
                <div class="path-step">
                  <span class="path-connector">│</span>
                  <span class="path-arrow">→</span>
                  <span class="tree-icon obj">◆</span>
                  <code>HashMap$Node</code>
                  <span class="field-name">.value</span>
                </div>
                <div class="path-step target">
                  <span class="path-connector">╰</span>
                  <span class="path-arrow">→</span>
                  <span class="tree-icon obj target">◆</span>
                  <code>com.app.model.OrderItem</code>
                  <span class="target-badge">TARGET</span>
                </div>
              </div>
            </div>
          </div>
          <div class="tree-node depth-0">
            <div class="tree-row root-path">
              <span class="tree-toggle">▶</span>
              <span class="tree-icon static">▣</span>
              <span class="tree-label"><code>Static: AppContext.instance</code> <span class="path-badge">shortest path: 6</span></span>
            </div>
          </div>
          <div class="tree-node depth-0">
            <div class="tree-row root-path">
              <span class="tree-toggle">▶</span>
              <span class="tree-icon thread">⊙</span>
              <span class="tree-label"><code>Thread: http-worker-1</code> <span class="path-badge">shortest path: 5</span></span>
            </div>
          </div>
        </div>
      </div>`;
  }
});
