/**
 * PerfScope — Goroutine Flame Graph View
 * Aggregated flame graph of all goroutine stacks, showing where goroutines
 * are spending time (blocked or running). Similar to CPU flame graph but
 * represents goroutine count instead of CPU time.
 */
ViewRouter.register('goroutine', {
  id: 'flamegraph',
  label: 'Flame Graph',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="1" y="6" width="12" height="7" rx="1"/><path d="M3 6V3a4 4 0 0 1 8 0v3"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-search">
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" opacity="0.5"><circle cx="6" cy="6" r="5"/><path d="m10 10 3 3"/></svg>
          <input type="text" placeholder="Highlight: function name..." class="viz-search-input">
        </div>
        <div class="viz-controls">
          <button class="viz-btn" title="Reset zoom">Reset</button>
          <div class="viz-separator"></div>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#ef4444"></span>Blocked</span>
            <span class="legend-item"><span class="legend-dot" style="background:#f59e0b"></span>Waiting</span>
            <span class="legend-item"><span class="legend-dot" style="background:#10b981"></span>Running</span>
            <span class="legend-item"><span class="legend-dot" style="background:#6366f1"></span>Runtime</span>
          </div>
        </div>
      </div>
      <div class="flamegraph-container goroutine-flame">
        <div class="flame-row root"><div class="flame-frame runtime" style="width:100%"><span class="frame-text">all goroutines (1,847)</span></div></div>
        <div class="flame-row">
          <div class="flame-frame blocked" style="width:42%"><span class="frame-text">runtime.gopark (780)</span></div>
          <div class="flame-frame waiting" style="width:35%"><span class="frame-text">runtime.netpoll (648)</span></div>
          <div class="flame-frame running" style="width:15%"><span class="frame-text">runtime.execute (275)</span></div>
          <div class="flame-frame runtime" style="width:8%"><span class="frame-text">runtime.schedule (144)</span></div>
        </div>
        <div class="flame-row">
          <div class="flame-frame blocked hot" style="width:22%"><span class="frame-text">runtime.chanrecv (412)</span></div>
          <div class="flame-frame blocked" style="width:12%"><span class="frame-text">sync.(*Mutex).Lock (218)</span></div>
          <div class="flame-frame blocked" style="width:8%"><span class="frame-text">runtime.semacquire (150)</span></div>
          <div class="flame-frame waiting" style="width:20%"><span class="frame-text">net.(*conn).Read (370)</span></div>
          <div class="flame-frame waiting" style="width:15%"><span class="frame-text">runtime.selectgo (278)</span></div>
          <div class="flame-frame running" style="width:15%"><span class="frame-text">main.handleRequest (275)</span></div>
          <div class="flame-frame runtime dim" style="width:8%"><span class="frame-text">runtime.gcBgMarkWorker (144)</span></div>
        </div>
        <div class="flame-row">
          <div class="flame-frame blocked very-hot" style="width:14%"><span class="frame-text">🔥 queue.(*Queue).Dequeue (256)</span></div>
          <div class="flame-frame blocked" style="width:8%"><span class="frame-text">pool.(*Pool).getConn (156)</span></div>
          <div class="flame-frame blocked" style="width:12%"><span class="frame-text">cache.(*Store).Get (218)</span></div>
          <div class="flame-frame blocked" style="width:8%"><span class="frame-text">db.(*Pool).Acquire (150)</span></div>
          <div class="flame-frame waiting" style="width:20%"><span class="frame-text">http.(*conn).serve (370)</span></div>
          <div class="flame-frame waiting" style="width:15%"><span class="frame-text">grpc.(*Server).Serve (278)</span></div>
          <div class="flame-frame running" style="width:15%"><span class="frame-text">service.ProcessOrder (275)</span></div>
          <div class="flame-frame runtime dim" style="width:8%"><span class="frame-text">runtime.scanobject (144)</span></div>
        </div>
        <div class="flame-row">
          <div class="flame-frame blocked very-hot" style="width:14%"><span class="frame-text">worker.(*Pool).processTask (256)</span></div>
          <div class="flame-frame blocked" style="width:8%"><span class="frame-text">redis.(*Client).Do (156)</span></div>
          <div class="flame-frame blocked" style="width:12%"><span class="frame-text">sync.(*RWMutex).RLock (218)</span></div>
          <div class="flame-frame blocked" style="width:8%"><span class="frame-text">pgx.(*Pool).Acquire (150)</span></div>
        </div>
      </div>
      <div class="hot-functions">
        <div class="hf-header"><h4>🔥 Top Blocked Stacks</h4></div>
        <div class="hf-table">
          <div class="hf-row header"><span class="hf-col rank">#</span><span class="hf-col name">Block Point</span><span class="hf-col self">Count</span><span class="hf-col total">% Total</span><span class="hf-col bar">Distribution</span></div>
          <div class="hf-row"><span class="hf-col rank">1</span><span class="hf-col name"><code>queue.(*Queue).Dequeue</code> — chan recv</span><span class="hf-col self hot">256</span><span class="hf-col total">13.9%</span><span class="hf-col bar"><div class="bar-fill very-hot" style="width:14%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">2</span><span class="hf-col name"><code>cache.(*Store).Get</code> — mutex lock</span><span class="hf-col self warn">218</span><span class="hf-col total">11.8%</span><span class="hf-col bar"><div class="bar-fill hot" style="width:12%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">3</span><span class="hf-col name"><code>redis.(*Client).Do</code> — pool wait</span><span class="hf-col self">156</span><span class="hf-col total">8.4%</span><span class="hf-col bar"><div class="bar-fill" style="width:8%"></div></span></div>
          <div class="hf-row"><span class="hf-col rank">4</span><span class="hf-col name"><code>db.(*Pool).Acquire</code> — semaphore</span><span class="hf-col self">150</span><span class="hf-col total">8.1%</span><span class="hf-col bar"><div class="bar-fill" style="width:8%"></div></span></div>
        </div>
      </div>`;
  }
});
