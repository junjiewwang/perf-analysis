/**
 * PerfScope — CPU Flame Graph View
 * Renders an interactive flame graph visualization for CPU profiling data.
 */
ViewRouter.register('cpu', {
  id: 'flamegraph',
  label: 'Flame Graph',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1" y="6" width="12" height="7" rx="1"/><path d="M3 6V3a4 4 0 0 1 8 0v3"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Highlight: type function name..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn" title="Reset zoom"><svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 7h12M7 1v12"/></svg>Reset</button>
          <button class="viz-btn" title="Reverse"><svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M7 1v12m0-12L4 4m3-3l3 3m-3 9L4 10m3 3l3-3"/></svg>Reverse</button>
          <div class="viz-separator"></div>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#ff6b35"></span>Application</span>
            <span class="legend-item"><span class="legend-dot" style="background:#10b981"></span>Runtime</span>
            <span class="legend-item"><span class="legend-dot" style="background:#6366f1"></span>Kernel</span>
            <span class="legend-item"><span class="legend-dot" style="background:#f59e0b"></span>GC</span>
          </div>
        </div>
      </div>
      <div class="flamegraph-container">
        <div class="flame-row root"><div class="flame-frame app" style="width:100%" data-name="all" data-pct="100%"><span class="frame-text">all (100%)</span></div></div>
        <div class="flame-row"><div class="flame-frame app" style="width:62%"><span class="frame-text">main.handleRequest (62%)</span></div><div class="flame-frame runtime" style="width:23%"><span class="frame-text">runtime.gcBgMarkWorker (23%)</span></div><div class="flame-frame kernel" style="width:15%"><span class="frame-text">syscall.Read (15%)</span></div></div>
        <div class="flame-row"><div class="flame-frame app hot" style="width:38%"><span class="frame-text">service.ProcessOrder (38%)</span></div><div class="flame-frame app" style="width:24%"><span class="frame-text">middleware.Auth (24%)</span></div><div class="flame-frame gc" style="width:23%"><span class="frame-text">runtime.scanobject (23%)</span></div><div class="flame-frame kernel" style="width:15%"><span class="frame-text">net.(*conn).Read (15%)</span></div></div>
        <div class="flame-row"><div class="flame-frame app very-hot" style="width:28%"><span class="frame-text">🔥 json.Marshal (28%)</span></div><div class="flame-frame app" style="width:10%"><span class="frame-text">db.Query (10%)</span></div><div class="flame-frame app" style="width:14%"><span class="frame-text">crypto/tls.Verify (14%)</span></div><div class="flame-frame app" style="width:10%"><span class="frame-text">jwt.Parse (10%)</span></div><div class="flame-frame gc" style="width:23%"><span class="frame-text">runtime.mallocgc (23%)</span></div><div class="flame-frame kernel dim" style="width:15%"><span class="frame-text">epoll_wait (15%)</span></div></div>
        <div class="flame-row"><div class="flame-frame app very-hot" style="width:18%"><span class="frame-text">reflect.Value.call (18%)</span></div><div class="flame-frame app" style="width:10%"><span class="frame-text">encoding.encodeStruct (10%)</span></div><div class="flame-frame app" style="width:10%"><span class="frame-text">sql.(*DB).queryContext (10%)</span></div><div class="flame-frame app" style="width:14%"><span class="frame-text">x509.parseCertificate (14%)</span></div><div class="flame-frame app" style="width:10%"><span class="frame-text">hmac.verify (10%)</span></div><div class="flame-frame gc dim" style="width:23%"><span class="frame-text">runtime.heapBits (23%)</span></div><div class="flame-frame kernel dim" style="width:15%"><span class="frame-text">futex (15%)</span></div></div>
        <div class="flame-row"><div class="flame-frame app" style="width:12%"><span class="frame-text">reflect.call (12%)</span></div><div class="flame-frame app" style="width:6%"><span class="frame-text">fmt.Sprintf (6%)</span></div><div class="flame-frame app dim" style="width:10%"><span class="frame-text">...</span></div></div>
      </div>
      <div class="hot-functions">
        <div class="hf-header"><h4>🔥 Hot Functions</h4><div class="hf-tabs"><button class="hf-tab active">By Self Time</button><button class="hf-tab">By Total Time</button></div></div>
        <div class="hf-table">
          <div class="hf-row header"><span class="hf-col rank">#</span><span class="hf-col name">Function</span><span class="hf-col self">Self</span><span class="hf-col total">Total</span><span class="hf-col bar">Distribution</span></div>
          <div class="hf-row"><span class="hf-col rank">1</span><span class="hf-col name"><code>json.Marshal</code></span><span class="hf-col self hot">28.0%</span><span class="hf-col total">28.0%</span><span class="hf-col bar"><div class="bar-fill very-hot" style="width:28%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">2</span><span class="hf-col name"><code>runtime.scanobject</code></span><span class="hf-col self warn">18.2%</span><span class="hf-col total">23.0%</span><span class="hf-col bar"><div class="bar-fill hot" style="width:23%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">3</span><span class="hf-col name"><code>syscall.Read</code></span><span class="hf-col self">15.0%</span><span class="hf-col total">15.0%</span><span class="hf-col bar"><div class="bar-fill" style="width:15%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">4</span><span class="hf-col name"><code>crypto/tls.Verify</code></span><span class="hf-col self">14.0%</span><span class="hf-col total">14.0%</span><span class="hf-col bar"><div class="bar-fill" style="width:14%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">5</span><span class="hf-col name"><code>db.Query</code></span><span class="hf-col self">10.0%</span><span class="hf-col total">10.0%</span><span class="hf-col bar"><div class="bar-fill" style="width:10%"></div></span></div>
        </div>
      </div>`;
  }
});
