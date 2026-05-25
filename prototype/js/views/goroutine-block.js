/**
 * PerfScope — Goroutine Block Profile View
 * Shows blocking operations and their impact on goroutine throughput.
 * Visualizes lock contention, channel operations, and I/O waits.
 */
ViewRouter.register('goroutine', {
  id: 'block',
  label: 'Block Profile',
  icon: '<svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="2" y="2" width="10" height="10" rx="1"/><path d="M5 5l4 4M9 5l-4 4"/></svg>',
  render(container) {
    container.innerHTML = `
      <div class="viz-toolbar">
        <div class="viz-controls">
          <button class="viz-btn active">By Total Block Time</button>
          <button class="viz-btn">By Count</button>
          <button class="viz-btn">By Avg Duration</button>
          <div class="viz-separator"></div>
          <span class="viz-stat">Total block time: <strong>2,847.3s</strong></span>
          <span class="viz-stat">Unique sites: <strong>23</strong></span>
        </div>
      </div>
      <div class="block-profile-summary">
        <div class="block-type-card">
          <div class="block-type-bar" style="width:52%"></div>
          <div class="block-type-icon">🔒</div>
          <div class="block-type-info">
            <div class="block-type-name">Mutex Contention</div>
            <div class="block-type-detail">8 sites, 312 goroutines affected</div>
          </div>
          <div class="block-type-time hot">1,480.2s (52%)</div>
        </div>
        <div class="block-type-card">
          <div class="block-type-bar" style="width:28%"></div>
          <div class="block-type-icon">📡</div>
          <div class="block-type-info">
            <div class="block-type-name">Channel Operations</div>
            <div class="block-type-detail">5 sites, 256 goroutines affected</div>
          </div>
          <div class="block-type-time warn">797.2s (28%)</div>
        </div>
        <div class="block-type-card">
          <div class="block-type-bar" style="width:15%"></div>
          <div class="block-type-icon">💾</div>
          <div class="block-type-info">
            <div class="block-type-name">I/O Wait</div>
            <div class="block-type-detail">6 sites, 156 goroutines affected</div>
          </div>
          <div class="block-type-time">427.1s (15%)</div>
        </div>
        <div class="block-type-card">
          <div class="block-type-bar" style="width:5%"></div>
          <div class="block-type-icon">🔄</div>
          <div class="block-type-info">
            <div class="block-type-name">Semaphore / Select</div>
            <div class="block-type-detail">4 sites, 84 goroutines affected</div>
          </div>
          <div class="block-type-time">142.8s (5%)</div>
        </div>
      </div>
      <div class="block-sites-table">
        <div class="block-site-header">
          <span class="bs-col rank">#</span>
          <span class="bs-col name">Block Site</span>
          <span class="bs-col type">Type</span>
          <span class="bs-col count">Count</span>
          <span class="bs-col avg">Avg Wait</span>
          <span class="bs-col total">Total</span>
          <span class="bs-col bar">Impact</span>
        </div>
        <div class="block-site-row">
          <span class="bs-col rank">1</span>
          <span class="bs-col name"><code>cache.(*Store).Get</code><br><span class="bs-detail">sync.(*RWMutex).RLock → cache.go:45</span></span>
          <span class="bs-col type"><span class="type-badge mutex">Mutex</span></span>
          <span class="bs-col count">218</span>
          <span class="bs-col avg hot">4.2s</span>
          <span class="bs-col total very-hot">915.6s</span>
          <span class="bs-col bar"><div class="bar-fill very-hot" style="width:72%"></div></span>
        </div>
        <div class="block-site-row">
          <span class="bs-col rank">2</span>
          <span class="bs-col name"><code>queue.(*Queue).Dequeue</code><br><span class="bs-detail">runtime.chanrecv → queue.go:67</span></span>
          <span class="bs-col type"><span class="type-badge chan">Chan</span></span>
          <span class="bs-col count">256</span>
          <span class="bs-col avg warn">2.8s</span>
          <span class="bs-col total hot">716.8s</span>
          <span class="bs-col bar"><div class="bar-fill hot" style="width:56%"></div></span>
        </div>
        <div class="block-site-row">
          <span class="bs-col rank">3</span>
          <span class="bs-col name"><code>db.(*Pool).Acquire</code><br><span class="bs-detail">runtime.semacquire → pool.go:112</span></span>
          <span class="bs-col type"><span class="type-badge mutex">Sema</span></span>
          <span class="bs-col count">150</span>
          <span class="bs-col avg">3.1s</span>
          <span class="bs-col total warn">465.0s</span>
          <span class="bs-col bar"><div class="bar-fill warn" style="width:37%"></div></span>
        </div>
        <div class="block-site-row">
          <span class="bs-col rank">4</span>
          <span class="bs-col name"><code>redis.(*Client).Do</code><br><span class="bs-detail">pool wait → redis.go:89</span></span>
          <span class="bs-col type"><span class="type-badge io">I/O</span></span>
          <span class="bs-col count">156</span>
          <span class="bs-col avg">1.8s</span>
          <span class="bs-col total">280.8s</span>
          <span class="bs-col bar"><div class="bar-fill" style="width:22%"></div></span>
        </div>
        <div class="block-site-row">
          <span class="bs-col rank">5</span>
          <span class="bs-col name"><code>http.(*Transport).roundTrip</code><br><span class="bs-detail">net.(*conn).Write → transport.go:234</span></span>
          <span class="bs-col type"><span class="type-badge io">I/O</span></span>
          <span class="bs-col count">89</span>
          <span class="bs-col avg">1.2s</span>
          <span class="bs-col total">106.8s</span>
          <span class="bs-col bar"><div class="bar-fill" style="width:8%"></div></span>
        </div>
      </div>`;
  }
});
