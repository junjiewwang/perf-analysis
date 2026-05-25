/**
 * PerfScope — CPU Timeline View
 * Shows CPU activity over time, with thread swim lanes.
 */
ViewRouter.register('cpu', {
  id: 'timeline',
  label: 'Timeline',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 1v12h12"/><path d="M4 9l3-3 2 2 4-5"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-controls" style="width:100%;justify-content:space-between;">
          <div style="display:flex;align-items:center;gap:8px;">
            <button class="viz-btn">◀</button>
            <span style="font-size:11px;color:var(--color-text-secondary);">0.0s — 30.0s (30s window)</span>
            <button class="viz-btn">▶</button>
          </div>
          <div style="display:flex;align-items:center;gap:8px;">
            <button class="viz-btn active">Threads</button>
            <button class="viz-btn">CPU %</button>
            <button class="viz-btn">GC Pauses</button>
          </div>
        </div>
      </div>
      <div class="timeline-container">
        <!-- Time ruler -->
        <div class="timeline-ruler">
          <span class="ruler-mark" style="left:0%">0s</span>
          <span class="ruler-mark" style="left:16.6%">5s</span>
          <span class="ruler-mark" style="left:33.3%">10s</span>
          <span class="ruler-mark" style="left:50%">15s</span>
          <span class="ruler-mark" style="left:66.6%">20s</span>
          <span class="ruler-mark" style="left:83.3%">25s</span>
          <span class="ruler-mark" style="left:100%">30s</span>
        </div>
        <!-- Thread swim lanes -->
        <div class="timeline-lane">
          <div class="lane-label">main</div>
          <div class="lane-track">
            <div class="lane-activity app" style="left:5%;width:25%;" title="handleRequest"></div>
            <div class="lane-activity app hot" style="left:32%;width:18%;" title="ProcessOrder"></div>
            <div class="lane-activity app" style="left:55%;width:12%;" title="handleRequest"></div>
            <div class="lane-activity app" style="left:72%;width:20%;" title="ProcessOrder"></div>
          </div>
        </div>
        <div class="timeline-lane">
          <div class="lane-label">pool-worker-1</div>
          <div class="lane-track">
            <div class="lane-activity app" style="left:2%;width:8%;"></div>
            <div class="lane-activity app" style="left:15%;width:12%;"></div>
            <div class="lane-activity app hot" style="left:35%;width:22%;" title="json.Marshal"></div>
            <div class="lane-activity app" style="left:62%;width:15%;"></div>
            <div class="lane-activity app" style="left:82%;width:10%;"></div>
          </div>
        </div>
        <div class="timeline-lane">
          <div class="lane-label">pool-worker-2</div>
          <div class="lane-track">
            <div class="lane-activity app" style="left:8%;width:10%;"></div>
            <div class="lane-activity app" style="left:25%;width:8%;"></div>
            <div class="lane-activity app" style="left:40%;width:18%;"></div>
            <div class="lane-activity app" style="left:68%;width:22%;"></div>
          </div>
        </div>
        <div class="timeline-lane">
          <div class="lane-label">GC Worker</div>
          <div class="lane-track">
            <div class="lane-activity gc" style="left:10%;width:4%;"></div>
            <div class="lane-activity gc" style="left:30%;width:6%;" title="STW"></div>
            <div class="lane-activity gc" style="left:52%;width:3%;"></div>
            <div class="lane-activity gc" style="left:75%;width:5%;"></div>
            <div class="lane-activity gc" style="left:90%;width:4%;"></div>
          </div>
        </div>
        <div class="timeline-lane">
          <div class="lane-label">net/poller</div>
          <div class="lane-track">
            <div class="lane-activity kernel dim" style="left:0%;width:100%;opacity:0.2;"></div>
            <div class="lane-activity kernel" style="left:12%;width:3%;"></div>
            <div class="lane-activity kernel" style="left:28%;width:2%;"></div>
            <div class="lane-activity kernel" style="left:45%;width:4%;"></div>
            <div class="lane-activity kernel" style="left:60%;width:3%;"></div>
            <div class="lane-activity kernel" style="left:78%;width:2%;"></div>
          </div>
        </div>
        <!-- CPU utilization sparkline -->
        <div class="timeline-lane cpu-util">
          <div class="lane-label">CPU %</div>
          <div class="lane-track">
            <svg class="cpu-sparkline" viewBox="0 0 300 40" preserveAspectRatio="none">
              <polyline fill="none" stroke="var(--color-orange)" stroke-width="1.5"
                points="0,35 15,30 30,20 45,15 60,12 75,18 90,8 105,5 120,10 135,15 150,20 165,12 180,8 195,14 210,18 225,10 240,6 255,12 270,18 285,22 300,25"/>
              <polyline fill="rgba(255,107,53,0.1)" stroke="none"
                points="0,40 0,35 15,30 30,20 45,15 60,12 75,18 90,8 105,5 120,10 135,15 150,20 165,12 180,8 195,14 210,18 225,10 240,6 255,12 270,18 285,22 300,25 300,40"/>
            </svg>
          </div>
        </div>
      </div>`;
  }
});
