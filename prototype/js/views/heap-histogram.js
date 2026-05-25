/**
 * PerfScope — Heap Class Histogram View
 * Displays heap memory usage by class, sorted by retained size.
 * Shows instance count, shallow size, and retained size with visual bars.
 */
ViewRouter.register('heap', {
  id: 'histogram',
  label: 'Class Histogram',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1" y="10" width="3" height="3" rx="0.5"/><rect x="5.5" y="6" width="3" height="7" rx="0.5"/><rect x="10" y="2" width="3" height="11" rx="0.5"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter: class name..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active" title="Sort by retained size">Retained</button>
          <button class="viz-btn" title="Sort by shallow size">Shallow</button>
          <button class="viz-btn" title="Sort by instance count">Count</button>
          <div class="viz-separator"></div>
          <span class="viz-stat">Total: <strong>1,247 classes</strong></span>
          <span class="viz-stat">Heap: <strong>512.3 MB</strong></span>
        </div>
      </div>
      <div class="histogram-table">
        <div class="hist-row header">
          <span class="hist-col rank">#</span>
          <span class="hist-col name">Class Name</span>
          <span class="hist-col count">Instances</span>
          <span class="hist-col shallow">Shallow Size</span>
          <span class="hist-col retained">Retained Size</span>
          <span class="hist-col bar">Distribution</span>
        </div>
        <div class="hist-row expandable" data-class="byte[]">
          <span class="hist-col rank">1</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>byte[]</code></span>
          <span class="hist-col count">845,231</span>
          <span class="hist-col shallow">128.4 MB</span>
          <span class="hist-col retained very-hot">198.7 MB</span>
          <span class="hist-col bar"><div class="bar-fill very-hot" style="width:85%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="java.lang.String">
          <span class="hist-col rank">2</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>java.lang.String</code></span>
          <span class="hist-col count">623,108</span>
          <span class="hist-col shallow">89.3 MB</span>
          <span class="hist-col retained hot">142.1 MB</span>
          <span class="hist-col bar"><div class="bar-fill hot" style="width:61%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="java.util.HashMap$Node">
          <span class="hist-col rank">3</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>java.util.HashMap$Node</code></span>
          <span class="hist-col count">412,897</span>
          <span class="hist-col shallow">52.8 MB</span>
          <span class="hist-col retained warn">78.4 MB</span>
          <span class="hist-col bar"><div class="bar-fill warn" style="width:33%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="char[]">
          <span class="hist-col rank">4</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>char[]</code></span>
          <span class="hist-col count">389,102</span>
          <span class="hist-col shallow">45.1 MB</span>
          <span class="hist-col retained">45.1 MB</span>
          <span class="hist-col bar"><div class="bar-fill" style="width:19%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="com.app.model.OrderItem">
          <span class="hist-col rank">5</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>com.app.model.OrderItem</code><span class="tag leak-suspect">⚠ Leak Suspect</span></span>
          <span class="hist-col count">234,567</span>
          <span class="hist-col shallow">28.9 MB</span>
          <span class="hist-col retained warn">38.2 MB</span>
          <span class="hist-col bar"><div class="bar-fill warn" style="width:16%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="java.util.ArrayList">
          <span class="hist-col rank">6</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>java.util.ArrayList</code></span>
          <span class="hist-col count">178,443</span>
          <span class="hist-col shallow">12.4 MB</span>
          <span class="hist-col retained">34.8 MB</span>
          <span class="hist-col bar"><div class="bar-fill" style="width:15%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="java.lang.Object[]">
          <span class="hist-col rank">7</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>java.lang.Object[]</code></span>
          <span class="hist-col count">156,789</span>
          <span class="hist-col shallow">22.1 MB</span>
          <span class="hist-col retained">31.5 MB</span>
          <span class="hist-col bar"><div class="bar-fill" style="width:13%"></div></span>
        </div>
        <div class="hist-row expandable" data-class="com.app.cache.CacheEntry">
          <span class="hist-col rank">8</span>
          <span class="hist-col name"><span class="expand-icon">▶</span><code>com.app.cache.CacheEntry</code><span class="tag leak-suspect">⚠ Leak Suspect</span></span>
          <span class="hist-col count">98,234</span>
          <span class="hist-col shallow">18.7 MB</span>
          <span class="hist-col retained warn">28.9 MB</span>
          <span class="hist-col bar"><div class="bar-fill warn" style="width:12%"></div></span>
        </div>
      </div>
      <div class="histogram-footer">
        <span class="hist-pagination">Showing 1–8 of 1,247 classes</span>
        <div class="hist-page-controls">
          <button class="viz-btn" disabled>← Prev</button>
          <button class="viz-btn">Next →</button>
        </div>
      </div>`;
  }
});
