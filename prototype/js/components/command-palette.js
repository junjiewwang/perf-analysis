/**
 * PerfScope — Command Palette Component
 * 
 * Provides quick-search functionality across functions, classes, goroutines.
 * Triggered via ⌘K or clicking the search bar.
 * 
 * Note: Currently a UI shell — will connect to real data in React migration.
 */

const CommandPalette = (function() {
  'use strict';

  const sampleResults = [
    { type: 'function', icon: 'ƒ', name: 'json.Marshal', detail: '28.0% CPU · encoding/json', badge: 'hot' },
    { type: 'function', icon: 'ƒ', name: 'runtime.scanobject', detail: '18.2% CPU · runtime', badge: 'warn' },
    { type: 'class', icon: '◆', name: 'byte[]', detail: '89.4 MB retained · 342K instances', badge: '' },
    { type: 'class', icon: '◆', name: 'java.lang.String', detail: '52.3 MB retained · 287K instances', badge: '' },
    { type: 'goroutine', icon: '⊙', name: 'sync.(*Mutex).Lock', detail: '184 blocked · max 42.3s', badge: 'hot' },
    { type: 'function', icon: 'ƒ', name: 'service.ProcessOrder', detail: '38.0% CPU · com/app/service', badge: '' },
    { type: 'goroutine', icon: '⊙', name: 'runtime.chanrecv1', detail: '281 waiting · chan recv', badge: '' },
  ];

  function filterResults(query) {
    if (!query) return sampleResults.slice(0, 5);
    const lower = query.toLowerCase();
    return sampleResults.filter(r => 
      r.name.toLowerCase().includes(lower) || 
      r.detail.toLowerCase().includes(lower)
    );
  }

  function renderResults(container, query) {
    const results = filterResults(query);
    if (results.length === 0) {
      container.innerHTML = '<div class="cmd-empty">No results found</div>';
      return;
    }

    container.innerHTML = results.map(r => `
      <div class="cmd-result" data-type="${r.type}">
        <span class="cmd-result-icon ${r.type}">${r.icon}</span>
        <div class="cmd-result-info">
          <span class="cmd-result-name">${r.name}</span>
          <span class="cmd-result-detail">${r.detail}</span>
        </div>
        ${r.badge ? `<span class="cmd-result-badge ${r.badge}">${r.badge.toUpperCase()}</span>` : ''}
      </div>
    `).join('');
  }

  function init() {
    const overlay = document.getElementById('commandOverlay');
    if (!overlay) return;

    // Add results container if not exists
    const bar = overlay.querySelector('.command-bar');
    if (bar && !bar.querySelector('.cmd-results')) {
      const resultsDiv = document.createElement('div');
      resultsDiv.className = 'cmd-results';
      bar.appendChild(resultsDiv);
      
      const input = bar.querySelector('.command-input');
      if (input) {
        input.addEventListener('input', function() {
          renderResults(resultsDiv, this.value);
        });
        // Show initial results when opened
        const observer = new MutationObserver(function() {
          if (overlay.classList.contains('visible')) {
            renderResults(resultsDiv, input.value);
          }
        });
        observer.observe(overlay, { attributes: true, attributeFilter: ['class'] });
      }
    }
  }

  // Auto-init on DOM ready
  document.addEventListener('DOMContentLoaded', init);

  return { filterResults, renderResults };
})();
