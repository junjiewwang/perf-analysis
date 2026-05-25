/**
 * PerfScope — Goroutine List View
 * Displays all goroutines grouped by state and stack signature.
 * Highlights blocked/waiting goroutines and their wait durations.
 */
ViewRouter.register('goroutine', {
  id: 'list',
  label: 'Goroutine List',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2 3h10M2 7h10M2 11h7"/><circle cx="12" cy="11" r="1" fill="currentColor"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Filter: function name or goroutine ID..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn active">By State</button>
          <button class="viz-btn">By Stack</button>
          <button class="viz-btn">By Duration</button>
          <div class="viz-separator"></div>
          <span class="viz-stat">Total: <strong>1,847</strong> goroutines</span>
        </div>
      </div>
      <div class="goroutine-state-summary">
        <div class="state-chip running"><span class="state-dot"></span>Running <strong>4</strong></div>
        <div class="state-chip runnable"><span class="state-dot"></span>Runnable <strong>23</strong></div>
        <div class="state-chip waiting"><span class="state-dot"></span>Waiting <strong>1,456</strong></div>
        <div class="state-chip blocked"><span class="state-dot"></span>Blocked <strong>312</strong></div>
        <div class="state-chip dead"><span class="state-dot"></span>Dead <strong>52</strong></div>
      </div>
      <div class="goroutine-groups">
        <div class="gr-group expanded">
          <div class="gr-group-header">
            <span class="expand-icon">▼</span>
            <span class="state-badge blocked">BLOCKED</span>
            <span class="gr-group-title">chan receive — <code>(*Queue).Dequeue</code></span>
            <span class="gr-group-count">128 goroutines</span>
            <span class="gr-group-duration hot">avg wait: 45.2s</span>
          </div>
          <div class="gr-group-body">
            <div class="gr-stack-trace">
              <div class="stack-frame highlight"><span class="frame-num">0</span><code>runtime.gopark</code> <span class="file-ref">proc.go:398</span></div>
              <div class="stack-frame highlight"><span class="frame-num">1</span><code>runtime.chanrecv</code> <span class="file-ref">chan.go:583</span></div>
              <div class="stack-frame"><span class="frame-num">2</span><code>com/app/queue.(*Queue).Dequeue</code> <span class="file-ref">queue.go:67</span></div>
              <div class="stack-frame"><span class="frame-num">3</span><code>com/app/worker.(*Pool).processTask</code> <span class="file-ref">pool.go:134</span></div>
              <div class="stack-frame"><span class="frame-num">4</span><code>com/app/worker.(*Pool).Run</code> <span class="file-ref">pool.go:89</span></div>
            </div>
            <div class="gr-instances">
              <span class="gr-id">#1024</span><span class="gr-id">#1025</span><span class="gr-id">#1026</span>
              <span class="gr-id">#1027</span><span class="gr-id">#1028</span>
              <span class="gr-more">+123 more...</span>
            </div>
          </div>
        </div>
        <div class="gr-group">
          <div class="gr-group-header">
            <span class="expand-icon">▶</span>
            <span class="state-badge blocked">BLOCKED</span>
            <span class="gr-group-title">mutex lock — <code>(*sync.Mutex).Lock</code></span>
            <span class="gr-group-count">84 goroutines</span>
            <span class="gr-group-duration warn">avg wait: 12.8s</span>
          </div>
        </div>
        <div class="gr-group">
          <div class="gr-group-header">
            <span class="expand-icon">▶</span>
            <span class="state-badge waiting">WAITING</span>
            <span class="gr-group-title">I/O wait — <code>net.(*conn).Read</code></span>
            <span class="gr-group-count">456 goroutines</span>
            <span class="gr-group-duration">avg wait: 2.1s</span>
          </div>
        </div>
        <div class="gr-group">
          <div class="gr-group-header">
            <span class="expand-icon">▶</span>
            <span class="state-badge waiting">WAITING</span>
            <span class="gr-group-title">select — <code>(*Server).handleConns</code></span>
            <span class="gr-group-count">892 goroutines</span>
            <span class="gr-group-duration">idle</span>
          </div>
        </div>
        <div class="gr-group">
          <div class="gr-group-header">
            <span class="expand-icon">▶</span>
            <span class="state-badge running">RUNNING</span>
            <span class="gr-group-title"><code>main.main</code></span>
            <span class="gr-group-count">4 goroutines</span>
            <span class="gr-group-duration">active</span>
          </div>
        </div>
      </div>`;
  }
});
