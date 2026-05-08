/**
 * PerfScope — Interactions & Panel Switching
 * Full prototype interactivity: session switching, profile switching, tab switching
 */

(function() {
  'use strict';

  // === State ===
  let currentPanel = 'cpu';

  // === DOM References ===
  const overlay = document.getElementById('commandOverlay');
  const trigger = document.getElementById('searchTrigger');
  const panels = {
    cpu: document.getElementById('panel-cpu'),
    heap: document.getElementById('panel-heap'),
    goroutine: document.getElementById('panel-goroutine'),
  };
  const contexts = {
    cpu: document.getElementById('context-cpu'),
    heap: document.getElementById('context-heap'),
    goroutine: document.getElementById('context-goroutine'),
  };
  const breadcrumbSession = document.getElementById('breadcrumbSession');

  // Session metadata for status bar updates
  const sessionMeta = {
    cpu: {
      breadcrumb: 'cpu-profile-2026-05-08T10:30',
      profile: 'Profile: CPU (30s @ 99Hz)',
      samples: '42,847 samples',
      frames: '1,284 unique frames',
    },
    heap: {
      breadcrumb: 'heap-dump-2026-05-08T10:32',
      profile: 'Profile: Heap Dump (256MB)',
      samples: '1,247,893 objects',
      frames: '3,412 classes',
    },
    goroutine: {
      breadcrumb: 'goroutine-dump-2026-05-08T10:35',
      profile: 'Profile: Goroutine (847 active)',
      samples: '847 goroutines',
      frames: '142 unique stacks',
    },
  };

  // === Panel Switching ===
  function switchPanel(type) {
    if (!panels[type]) return;
    currentPanel = type;

    // Switch main panels
    Object.entries(panels).forEach(([key, el]) => {
      el.style.display = key === type ? 'flex' : 'none';
    });

    // Switch context panels
    Object.entries(contexts).forEach(([key, el]) => {
      el.style.display = key === type ? 'block' : 'none';
    });

    // Update profile selector buttons
    document.querySelectorAll('.profile-btn').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.type === type);
    });

    // Update breadcrumb
    if (breadcrumbSession && sessionMeta[type]) {
      breadcrumbSession.textContent = sessionMeta[type].breadcrumb;
    }

    // Update status bar
    const statusProfile = document.getElementById('statusProfile');
    const statusSamples = document.getElementById('statusSamples');
    const statusFrames = document.getElementById('statusFrames');
    if (statusProfile) statusProfile.textContent = sessionMeta[type].profile;
    if (statusSamples) statusSamples.textContent = sessionMeta[type].samples;
    if (statusFrames) statusFrames.textContent = sessionMeta[type].frames;

    // Trigger animations on new panel
    const activePanel = panels[type];
    activePanel.style.opacity = '0';
    activePanel.style.transform = 'translateY(4px)';
    requestAnimationFrame(() => {
      activePanel.style.transition = 'opacity 0.25s ease, transform 0.25s ease';
      activePanel.style.opacity = '1';
      activePanel.style.transform = 'translateY(0)';
    });
  }

  // === Command Palette (⌘K) ===
  function openCommand() {
    overlay.classList.add('visible');
    setTimeout(() => overlay.querySelector('.command-input').focus(), 50);
  }

  function closeCommand() {
    overlay.classList.remove('visible');
  }

  trigger.addEventListener('click', openCommand);
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closeCommand();
  });

  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      overlay.classList.contains('visible') ? closeCommand() : openCommand();
    }
    if (e.key === 'Escape') closeCommand();
  });

  // === Session List Click (Left Sidebar) ===
  document.querySelectorAll('.session-item').forEach(item => {
    item.addEventListener('click', () => {
      // Update active state
      document.querySelectorAll('.session-item').forEach(i => i.classList.remove('active'));
      item.classList.add('active');

      // Switch to corresponding panel
      const sessionType = item.dataset.session;
      if (sessionType) {
        switchPanel(sessionType);
      }
    });
  });

  // === Profile Selector (Top Bar) ===
  document.querySelectorAll('.profile-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const type = btn.dataset.type;
      if (type) {
        switchPanel(type);
        // Also highlight corresponding session in sidebar
        document.querySelectorAll('.session-item').forEach(item => {
          if (item.dataset.session === type && !item.classList.contains('active')) {
            document.querySelectorAll('.session-item').forEach(i => i.classList.remove('active'));
            item.classList.add('active');
          }
        });
      }
    });
  });

  // === View Tab Switching (within each panel) ===
  document.querySelectorAll('.view-tabs').forEach(tabBar => {
    tabBar.querySelectorAll('.view-tab').forEach(tab => {
      tab.addEventListener('click', () => {
        tabBar.querySelectorAll('.view-tab').forEach(t => t.classList.remove('active'));
        tab.classList.add('active');
      });
    });
  });

  // === Hot Functions Tab Switching ===
  document.querySelectorAll('.hf-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      const parent = tab.closest('.hf-tabs');
      parent.querySelectorAll('.hf-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
    });
  });

  // === Flame Frame Hover Tooltip ===
  const tooltip = document.getElementById('flameTooltip');
  if (tooltip) {
    document.querySelectorAll('.flame-frame').forEach(frame => {
      frame.addEventListener('mouseenter', () => {
        const name = frame.dataset.name || frame.querySelector('.frame-text')?.textContent || '';
        const pct = frame.dataset.pct || '';
        if (name && name !== '...') {
          tooltip.querySelector('.tooltip-header').textContent = name;
          tooltip.style.display = 'block';
        }
      });
      frame.addEventListener('mouseleave', () => {
        tooltip.style.display = 'none';
      });
    });
  }

  // === Goroutine Group Collapse/Expand ===
  document.querySelectorAll('.gr-group-header').forEach(header => {
    header.addEventListener('click', () => {
      const group = header.closest('.gr-group');
      group.classList.toggle('collapsed');
      const expand = header.querySelector('.gr-group-expand');
      if (expand) {
        expand.textContent = group.classList.contains('collapsed') ? '▶' : '▼';
      }
    });
  });

  // === Viz Button Toggle ===
  document.querySelectorAll('.viz-controls').forEach(controls => {
    controls.querySelectorAll('.viz-btn').forEach(btn => {
      btn.addEventListener('click', () => {
        // Only toggle within same group (if it's a toggle group)
        const siblings = controls.querySelectorAll('.viz-btn');
        if (siblings.length > 1) {
          siblings.forEach(b => b.classList.remove('active'));
          btn.classList.add('active');
        }
      });
    });
  });

  // === Init ===
  console.log(
    '%c🔥 PerfScope Prototype',
    'color: #ff6b35; font-size: 14px; font-weight: bold;'
  );
  console.log(
    '%c  ⌘K — Quick Search  |  Click sessions or profile buttons to switch views',
    'color: #9ba1b8; font-size: 11px;'
  );

})();
