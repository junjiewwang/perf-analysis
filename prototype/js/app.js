/**
 * PerfScope — Application Entry Point
 * 
 * Navigation model: Session (left sidebar) → View (tab bar)
 * - Left sidebar sessions drive panel switching
 * - Each session has a type (cpu/heap/goroutine) that determines the panel
 * - Breadcrumb updates to reflect current session
 * - No duplicate "profile type selector" — single source of truth
 */

(function() {
  'use strict';

  document.addEventListener('DOMContentLoaded', function() {
    // Initialize ViewRouter — builds tabs and renders default views
    ViewRouter.init();

    // Wire session list clicks — this is the ONLY panel switching mechanism
    const sessionItems = document.querySelectorAll('.session-item');
    sessionItems.forEach(item => {
      item.addEventListener('click', function() {
        const session = this.dataset.session;
        
        // Update active state
        sessionItems.forEach(s => s.classList.remove('active'));
        this.classList.add('active');

        // Switch panel via ViewRouter
        ViewRouter.switchPanel(session);

        // Update breadcrumb to reflect selected session
        updateBreadcrumb(this);
      });
    });

    // Command palette (⌘K)
    const overlay = document.getElementById('commandOverlay');
    const searchTrigger = document.getElementById('searchTrigger');
    
    if (searchTrigger && overlay) {
      searchTrigger.addEventListener('click', () => toggleCommandPalette(true));
      
      overlay.addEventListener('click', function(e) {
        if (e.target === this) toggleCommandPalette(false);
      });

      document.addEventListener('keydown', function(e) {
        if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
          e.preventDefault();
          toggleCommandPalette(!overlay.classList.contains('visible'));
        }
        if (e.key === 'Escape' && overlay.classList.contains('visible')) {
          toggleCommandPalette(false);
        }
      });
    }
  });

  function toggleCommandPalette(show) {
    const overlay = document.getElementById('commandOverlay');
    if (!overlay) return;
    
    if (show) {
      overlay.classList.add('visible');
      const input = overlay.querySelector('.command-input');
      if (input) {
        input.value = '';
        input.focus();
      }
    } else {
      overlay.classList.remove('visible');
    }
  }

  function updateBreadcrumb(sessionEl) {
    const breadcrumbSession = document.getElementById('breadcrumbSession');
    if (!breadcrumbSession) return;

    const name = sessionEl.querySelector('.session-name');
    const meta = sessionEl.querySelector('.session-meta');
    
    if (name && meta) {
      // Build breadcrumb from session info: "cpu-profile-10:30-30s"
      const type = sessionEl.dataset.session;
      const metaText = meta.textContent.trim();
      breadcrumbSession.textContent = `${name.textContent.toLowerCase().replace(/\s+/g, '-')}-${metaText.split('·')[0].trim()}`;
    }
  }
})();
