/**
 * Theme Manager - Unified theme management and switching
 * 
 * Features:
 * - Multiple theme support (light, dark, purple, ocean, sunset)
 * - System preference detection
 * - Persistent storage
 * - Smooth transitions
 * - Event-driven updates
 */

const ThemeManager = {
    // Available themes
    themes: [
        { id: 'light', name: 'Light', icon: '☀️', description: 'Default light theme' },
        { id: 'dark', name: 'Dark', icon: '🌙', description: 'Dark mode for low-light environments' },
        { id: 'purple', name: 'Purple', icon: '💜', description: 'Vibrant purple accent' },
        { id: 'ocean', name: 'Ocean', icon: '🌊', description: 'Cool blue tones' },
        { id: 'sunset', name: 'Sunset', icon: '🌅', description: 'Warm orange and rose' }
    ],
    
    // Storage key
    storageKey: 'perf-analysis-theme',
    
    // Current theme
    currentTheme: 'light',
    
    // Event listeners
    listeners: [],
    
    /**
     * Initialize theme manager
     */
    init() {
        // Get saved theme or detect system preference
        const saved = localStorage.getItem(this.storageKey);
        const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
        const initialTheme = saved || (prefersDark ? 'dark' : 'light');
        
        // Apply initial theme without transition
        this.setTheme(initialTheme, false);
        
        // Listen for system preference changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            // Only auto-switch if user hasn't manually selected a theme
            if (!localStorage.getItem(this.storageKey)) {
                this.setTheme(e.matches ? 'dark' : 'light');
            }
        });
        
        // Expose to window for debugging
        window.ThemeManager = this;
        
        console.log('[ThemeManager] Initialized with theme:', initialTheme);
    },
    
    /**
     * Set the current theme
     * @param {string} themeId - Theme identifier
     * @param {boolean} animate - Whether to animate the transition
     */
    setTheme(themeId, animate = true) {
        // Validate theme
        if (!this.themes.find(t => t.id === themeId)) {
            console.warn('[ThemeManager] Invalid theme:', themeId);
            themeId = 'light';
        }
        
        const html = document.documentElement;
        
        // Add transition class for smooth switching
        if (animate) {
            html.classList.add('theme-transitioning');
        }
        
        // Set theme attribute
        html.setAttribute('data-theme', themeId);
        this.currentTheme = themeId;
        
        // Save to storage
        localStorage.setItem(this.storageKey, themeId);
        
        // Update UI elements
        this.updateUI();
        
        // Notify listeners
        this.notifyListeners(themeId);
        
        // Remove transition class after animation
        if (animate) {
            setTimeout(() => {
                html.classList.remove('theme-transitioning');
            }, 300);
        }
        
        console.log('[ThemeManager] Theme changed to:', themeId);
    },
    
    /**
     * Get current theme
     * @returns {object} Current theme object
     */
    getTheme() {
        return this.themes.find(t => t.id === this.currentTheme) || this.themes[0];
    },
    
    /**
     * Get all available themes
     * @returns {array} Array of theme objects
     */
    getThemes() {
        return this.themes;
    },
    
    /**
     * Toggle to next theme in list
     */
    toggle() {
        const currentIdx = this.themes.findIndex(t => t.id === this.currentTheme);
        const nextIdx = (currentIdx + 1) % this.themes.length;
        this.setTheme(this.themes[nextIdx].id);
    },
    
    /**
     * Toggle between light and dark only
     */
    toggleDarkMode() {
        this.setTheme(this.currentTheme === 'dark' ? 'light' : 'dark');
    },
    
    /**
     * Check if current theme is dark
     * @returns {boolean}
     */
    isDark() {
        return this.currentTheme === 'dark';
    },
    
    /**
     * Update UI elements (icons, buttons, etc.)
     */
    updateUI() {
        const theme = this.getTheme();
        
        // Update theme toggle button icon
        const iconEl = document.getElementById('themeIcon');
        if (iconEl) {
            iconEl.textContent = theme.icon;
        }
        
        // Update theme name display
        const nameEl = document.getElementById('themeName');
        if (nameEl) {
            nameEl.textContent = theme.name;
        }
        
        // Update dropdown selection
        const dropdown = document.getElementById('themeSelect');
        if (dropdown) {
            dropdown.value = theme.id;
        }
        
        // Update active state in theme picker
        document.querySelectorAll('[data-theme-option]').forEach(el => {
            const isActive = el.dataset.themeOption === theme.id;
            el.classList.toggle('active', isActive);
            el.setAttribute('aria-selected', isActive);
        });
    },
    
    /**
     * Add theme change listener
     * @param {function} callback - Callback function(themeId)
     */
    onChange(callback) {
        if (typeof callback === 'function') {
            this.listeners.push(callback);
        }
    },
    
    /**
     * Remove theme change listener
     * @param {function} callback - Callback to remove
     */
    offChange(callback) {
        this.listeners = this.listeners.filter(cb => cb !== callback);
    },
    
    /**
     * Notify all listeners of theme change
     * @param {string} themeId - New theme ID
     */
    notifyListeners(themeId) {
        this.listeners.forEach(cb => {
            try {
                cb(themeId);
            } catch (e) {
                console.error('[ThemeManager] Listener error:', e);
            }
        });
    },
    
    /**
     * Get CSS variable value
     * @param {string} varName - Variable name (without --)
     * @returns {string} CSS variable value
     */
    getCSSVar(varName) {
        return getComputedStyle(document.documentElement)
            .getPropertyValue(`--color-${varName}`)
            .trim();
    },
    
    /**
     * Get color as RGB string for use in JS
     * @param {string} colorName - Color name (e.g., 'primary', 'bg-base')
     * @returns {string} RGB color string
     */
    getColor(colorName) {
        const rgb = this.getCSSVar(colorName);
        return rgb ? `rgb(${rgb})` : null;
    },
    
    /**
     * Get color as hex for libraries that need it
     * @param {string} colorName - Color name
     * @returns {string} Hex color string
     */
    getColorHex(colorName) {
        const rgb = this.getCSSVar(colorName);
        if (!rgb) return null;
        
        const [r, g, b] = rgb.split(' ').map(Number);
        return '#' + [r, g, b].map(x => x.toString(16).padStart(2, '0')).join('');
    },
    
    /**
     * Create theme picker dropdown HTML
     * @returns {string} HTML string
     */
    createPickerHTML() {
        return `
            <div class="theme-picker">
                <button class="theme-picker-trigger" onclick="ThemeManager.togglePicker()" title="Change theme">
                    <span id="themeIcon">${this.getTheme().icon}</span>
                    <span class="sr-only">Theme</span>
                </button>
                <div class="theme-picker-dropdown" id="themePickerDropdown">
                    ${this.themes.map(t => `
                        <button class="theme-option ${t.id === this.currentTheme ? 'active' : ''}" 
                                data-theme-option="${t.id}"
                                onclick="ThemeManager.setTheme('${t.id}')"
                                title="${t.description}">
                            <span class="theme-option-icon">${t.icon}</span>
                            <span class="theme-option-name">${t.name}</span>
                        </button>
                    `).join('')}
                </div>
            </div>
        `;
    },
    
    /**
     * Toggle theme picker dropdown visibility
     */
    togglePicker() {
        const dropdown = document.getElementById('themePickerDropdown');
        if (dropdown) {
            dropdown.classList.toggle('show');
            
            // Close on outside click
            if (dropdown.classList.contains('show')) {
                const closeHandler = (e) => {
                    if (!e.target.closest('.theme-picker')) {
                        dropdown.classList.remove('show');
                        document.removeEventListener('click', closeHandler);
                    }
                };
                setTimeout(() => document.addEventListener('click', closeHandler), 0);
            }
        }
    }
};

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => ThemeManager.init());
} else {
    ThemeManager.init();
}
