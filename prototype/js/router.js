/**
 * PerfScope — View Router
 * 
 * Responsibilities:
 * - Register view modules per panel (cpu/heap/goroutine)
 * - Switch between views within a panel (tab switching)
 * - Switch between panels (profile type switching)
 * - Render view content into the designated container
 * 
 * Design: Each view is a self-contained module that exports { id, label, icon, render() }
 * The router orchestrates which view is mounted, unmounted, and visible.
 */

const ViewRouter = (function() {
  'use strict';

  // Registry: panelId -> [{ id, label, icon, render, destroy? }]
  const registry = {};
  
  // State
  let activePanel = 'cpu';
  let activeViews = {}; // panelId -> activeViewId

  /**
   * Register a view module for a panel
   * @param {string} panelId - 'cpu' | 'heap' | 'goroutine'
   * @param {object} viewModule - { id, label, icon, render(container), destroy?() }
   */
  function register(panelId, viewModule) {
    if (!registry[panelId]) {
      registry[panelId] = [];
    }
    registry[panelId].push(viewModule);
  }

  /**
   * Initialize the router - build tabs and render default views
   */
  function init() {
    Object.keys(registry).forEach(panelId => {
      buildTabs(panelId);
      // Activate first view by default
      const views = registry[panelId];
      if (views.length > 0) {
        activeViews[panelId] = views[0].id;
        if (panelId === activePanel) {
          activateView(panelId, views[0].id);
        }
      }
    });
  }

  /**
   * Build tab buttons for a panel
   */
  function buildTabs(panelId) {
    const tabBar = document.querySelector(`#panel-${panelId} .view-tabs .view-tabs-list`);
    if (!tabBar) return;

    const views = registry[panelId] || [];
    tabBar.innerHTML = '';

    views.forEach((view, index) => {
      const btn = document.createElement('button');
      btn.className = 'view-tab' + (index === 0 ? ' active' : '');
      btn.dataset.viewId = view.id;
      btn.innerHTML = `${view.icon}<span>${view.label}</span>`;
      btn.addEventListener('click', () => switchView(panelId, view.id));
      tabBar.appendChild(btn);
    });
  }

  /**
   * Switch to a specific view within a panel
   */
  function switchView(panelId, viewId) {
    if (activeViews[panelId] === viewId) return;

    // Destroy previous view if it has cleanup
    const prevView = getView(panelId, activeViews[panelId]);
    if (prevView && prevView.destroy) {
      prevView.destroy();
    }

    activeViews[panelId] = viewId;
    activateView(panelId, viewId);

    // Update tab active state
    const tabBar = document.querySelector(`#panel-${panelId} .view-tabs .view-tabs-list`);
    if (tabBar) {
      tabBar.querySelectorAll('.view-tab').forEach(tab => {
        tab.classList.toggle('active', tab.dataset.viewId === viewId);
      });
    }
  }

  /**
   * Activate (render) a view into its container
   */
  function activateView(panelId, viewId) {
    const container = document.querySelector(`#panel-${panelId} .view-content`);
    if (!container) return;

    const view = getView(panelId, viewId);
    if (!view) return;

    // Clear and render
    container.innerHTML = '';
    container.className = 'view-content view-' + viewId;
    view.render(container);

    // Animate in
    container.style.opacity = '0';
    container.style.transform = 'translateY(4px)';
    requestAnimationFrame(() => {
      container.style.transition = 'opacity 0.2s ease, transform 0.2s ease';
      container.style.opacity = '1';
      container.style.transform = 'translateY(0)';
    });
  }

  /**
   * Switch the active panel (cpu/heap/goroutine)
   */
  function switchPanel(panelId) {
    if (!registry[panelId]) return;
    activePanel = panelId;

    // Show/hide panels
    document.querySelectorAll('.analysis-area').forEach(el => {
      el.style.display = el.id === `panel-${panelId}` ? 'flex' : 'none';
    });

    // Show/hide context panels
    document.querySelectorAll('.context-body').forEach(el => {
      el.style.display = el.id === `context-${panelId}` ? 'block' : 'none';
    });

    // Render active view if not already rendered
    const viewId = activeViews[panelId];
    if (viewId) {
      activateView(panelId, viewId);
    }

    return panelId;
  }

  /**
   * Get a view module by panel and viewId
   */
  function getView(panelId, viewId) {
    const views = registry[panelId] || [];
    return views.find(v => v.id === viewId);
  }

  /**
   * Get current state
   */
  function getState() {
    return { activePanel, activeViews: { ...activeViews } };
  }

  return {
    register,
    init,
    switchView,
    switchPanel,
    getState,
  };
})();
