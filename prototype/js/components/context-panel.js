/**
 * PerfScope — Context Panel Component
 * 
 * Manages the right sidebar detail panel content per active panel type.
 * Shows function/class/goroutine details, callers, source, and AI insights.
 * 
 * Note: Currently displays static mock data — will be data-driven in React.
 */

const ContextPanel = (function() {
  'use strict';

  // Switch context content based on active panel
  function switchContext(panelId) {
    document.querySelectorAll('.context-body').forEach(el => {
      el.style.display = el.id === `context-${panelId}` ? 'block' : 'none';
    });
  }

  // Initialize — hook into ViewRouter panel switches
  function init() {
    // The ViewRouter.switchPanel already handles context switching
    // This component is a placeholder for future React migration
  }

  document.addEventListener('DOMContentLoaded', init);

  return { switchContext };
})();
