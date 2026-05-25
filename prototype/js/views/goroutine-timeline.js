/**
 * PerfScope — Goroutine Timeline View
 * Displays goroutine lifecycle over time as swim lanes.
 * Shows state transitions (running → blocked → waiting → running).
 */
ViewRouter.register('goroutine', {
  id: 'timeline',
  label: 'Timeline',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 3h12M1 7h12M1 11h12"/><rect x="2" y="2" width="4" height="2" rx="0.5" fill="currentColor" opacity="0.6"/><rect x="7" y="6" width="5" height="2" rx="0.5" fill="currentColor" opacity="0.6"/><rect x="3" y="10" width="3" height="2" rx="0.5" fill="currentColor" opacity="0.6"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-controls">
          <button class="viz-btn" title="Zoom in">🔍+</button>
          <button class="viz-btn" title="Zoom out">🔍−</button>
          <button class="viz-btn" title="Fit all">Fit</button>
          <div class="viz-separator"></div>
          <div class="legend-items">
            <span class="legend-item"><span class="legend-dot" style="background:#10b981"></span>Running</span>
            <span class="legend-item"><span class="legend-dot" style="background:#f59e0b"></span>Runnable</span>
            <span class="legend-item"><span class="legend-dot" style="background:#ef4444"></span>Blocked</span>
            <span class="legend-item"><span class="legend-dot" style="background:#94a3b8"></span>Waiting</span>
            <span class="legend-item"><span class="legend-dot" style="background:#8b5cf6"></span>Syscall</span>
          </div>
          <div class="viz-separator"></div>
          <span class="viz-stat">Duration: <strong>12.4s</strong></span>
          <span class="viz-stat">Showing: <strong>top 20 goroutines</strong></span>
        </div>
      </div>
      <div class="timeline-container goroutine-timeline">
        <div class="timeline-ruler">
          <span class="ruler-mark" style="left:0%">0s</span>
          <span class="ruler-mark" style="left:16.7%">2s</span>
          <span class="ruler-mark" style="left:33.3%">4s</span>
          <span class="ruler-mark" style="left:50%">6s</span>
          <span class="ruler-mark" style="left:66.7%">8s</span>
          <span class="ruler-mark" style="left:83.3%">10s</span>
          <span class="ruler-mark" style="left:100%">12.4s</span>
        </div>
        <div class="timeline-lanes">
          <div class="timeline-lane">
            <div class="lane-label"><code>main</code> <span class="gr-id-sm">#1</span></div>
            <div class="lane-track">
              <div class="lane-segment running" style="left:0%; width:100%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>worker.Process</code> <span class="gr-id-sm">#24</span></div>
            <div class="lane-track">
              <div class="lane-segment running" style="left:0%; width:15%"></div>
              <div class="lane-segment blocked" style="left:15%; width:35%" title="chan recv: 4.3s"></div>
              <div class="lane-segment running" style="left:50%; width:10%"></div>
              <div class="lane-segment blocked" style="left:60%; width:30%" title="chan recv: 3.7s"></div>
              <div class="lane-segment running" style="left:90%; width:10%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>http.Serve</code> <span class="gr-id-sm">#56</span></div>
            <div class="lane-track">
              <div class="lane-segment waiting" style="left:0%; width:40%"></div>
              <div class="lane-segment runnable" style="left:40%; width:5%"></div>
              <div class="lane-segment running" style="left:45%; width:20%"></div>
              <div class="lane-segment waiting" style="left:65%; width:35%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>cache.Refresh</code> <span class="gr-id-sm">#89</span></div>
            <div class="lane-track">
              <div class="lane-segment waiting" style="left:0%; width:20%"></div>
              <div class="lane-segment running" style="left:20%; width:8%"></div>
              <div class="lane-segment blocked" style="left:28%; width:42%" title="mutex: 5.2s"></div>
              <div class="lane-segment running" style="left:70%; width:12%"></div>
              <div class="lane-segment waiting" style="left:82%; width:18%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>db.Query</code> <span class="gr-id-sm">#112</span></div>
            <div class="lane-track">
              <div class="lane-segment waiting" style="left:0%; width:30%"></div>
              <div class="lane-segment running" style="left:30%; width:5%"></div>
              <div class="lane-segment syscall" style="left:35%; width:25%" title="I/O: 3.1s"></div>
              <div class="lane-segment running" style="left:60%; width:8%"></div>
              <div class="lane-segment syscall" style="left:68%; width:20%" title="I/O: 2.5s"></div>
              <div class="lane-segment running" style="left:88%; width:12%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>grpc.Stream</code> <span class="gr-id-sm">#145</span></div>
            <div class="lane-track">
              <div class="lane-segment running" style="left:0%; width:10%"></div>
              <div class="lane-segment waiting" style="left:10%; width:60%"></div>
              <div class="lane-segment running" style="left:70%; width:15%"></div>
              <div class="lane-segment waiting" style="left:85%; width:15%"></div>
            </div>
          </div>
          <div class="timeline-lane">
            <div class="lane-label"><code>redis.Ping</code> <span class="gr-id-sm">#203</span></div>
            <div class="lane-track">
              <div class="lane-segment waiting" style="left:0%; width:45%"></div>
              <div class="lane-segment running" style="left:45%; width:3%"></div>
              <div class="lane-segment syscall" style="left:48%; width:12%" title="net I/O"></div>
              <div class="lane-segment running" style="left:60%; width:5%"></div>
              <div class="lane-segment waiting" style="left:65%; width:35%"></div>
            </div>
          </div>
          <div class="timeline-lane dim">
            <div class="lane-label"><span class="gr-more-label">+13 more goroutines...</span></div>
            <div class="lane-track">
              <div class="lane-segment waiting" style="left:0%; width:100%"></div>
            </div>
          </div>
        </div>
      </div>
      <div class="timeline-stats">
        <div class="ts-card">
          <div class="ts-title">Processor Utilization</div>
          <div class="ts-sparkline">
            <svg viewBox="0 0 200 40" class="sparkline-svg">
              <polyline points="0,35 15,30 30,20 45,25 60,15 75,18 90,10 105,12 120,8 135,15 150,20 165,18 180,22 200,25" fill="none" stroke="#10b981" stroke-width="1.5"/>
              <polyline points="0,35 15,30 30,20 45,25 60,15 75,18 90,10 105,12 120,8 135,15 150,20 165,18 180,22 200,25" fill="url(#sparkGrad)" stroke="none" opacity="0.2"/>
            </svg>
          </div>
          <div class="ts-value">Avg: <strong>3.2 / 8 cores</strong> (40%)</div>
        </div>
        <div class="ts-card">
          <div class="ts-title">Goroutine Count Over Time</div>
          <div class="ts-sparkline">
            <svg viewBox="0 0 200 40" class="sparkline-svg">
              <polyline points="0,38 20,35 40,30 60,28 80,20 100,15 120,12 140,14 160,16 180,18 200,20" fill="none" stroke="#6366f1" stroke-width="1.5"/>
            </svg>
          </div>
          <div class="ts-value">Peak: <strong>1,847</strong> at t=8.2s</div>
        </div>
      </div>`;
  }
});
