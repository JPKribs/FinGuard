
// APPLICATION INITIALIZATION

class App {
    static isInitialized = false;

    // MARK: initializeManagers
    // Initialize all application managers
    static async initializeManagers() {
        if (this.isInitialized) return;
        
        try {
            // Initialize all managers
            if (window.ServicesManager) {
                window.ServicesManager.initialize();
            }
            
            if (window.StatusManager) {
                window.StatusManager.initialize();
            }
            
            if (window.LogsManager) {
                window.LogsManager.initialize();
            }
            
            if (window.TunnelsManager) {
                window.TunnelsManager.initialize();
            }
            
            if (window.UpdateManager) {
                window.UpdateManager.initialize();
            }
            
            // Load initial data for the active tab
            this.loadInitialData();
            
            this.isInitialized = true;
            console.log('All managers initialized successfully');
            
        } catch (error) {
            console.error('Failed to initialize managers:', error);
        }
    }

    // MARK: loadInitialData
    // Load data for the currently active tab
    static loadInitialData() {
        const activeTab = document.querySelector('.tab.active');
        if (!activeTab) return;
        
        const tabName = activeTab.textContent.trim().toLowerCase();
        
        switch (tabName) {
            case 'system':
                if (window.StatusManager) {
                    window.StatusManager.loadStatus();
                }
                break;
            case 'services':
                if (window.ServicesManager) {
                    window.ServicesManager.loadServices();
                }
                break;
            case 'tunnels':
                if (window.TunnelsManager) {
                    window.TunnelsManager.loadTunnels();
                }
                break;
            default:
                // Load status as fallback
                if (window.StatusManager) {
                    window.StatusManager.loadStatus();
                }
        }
    }

    // MARK: setupApplication
    // Set up application-wide features
    static setupApplication() {
        this.setupTabSwitching();
        this.setupGlobalKeyboardShortcuts();
        this.setupAlertDismissal();
        this.handleURLFragments();
    }

    // MARK: setupTabSwitching
    // Set up tab switching functionality with data loading
    static setupTabSwitching() {
        const originalShowTab = window.Utils?.showTab;
        if (originalShowTab) {
            window.Utils.showTab = function(tabName) {
                // Call original showTab function
                originalShowTab.call(this, tabName);
                
                // Load appropriate data after tab switch
                setTimeout(() => {
                    App.handleTabSwitch(tabName);
                }, 50);
            };
        }
    }

    // MARK: handleTabSwitch
    // Handle loading data when tabs are switched
    static handleTabSwitch(tabName) {
        switch (tabName) {
            case 'status':
                if (window.StatusManager) {
                    window.StatusManager.loadStatus();
                }
                break;
            case 'services':
                if (window.ServicesManager) {
                    window.ServicesManager.loadServices();
                }
                break;
            case 'tunnels':
                if (window.TunnelsManager) {
                    window.TunnelsManager.loadTunnels();
                }
                break;
            case 'logs':
                if (window.LogsManager) {
                    window.LogsManager.loadLogs(0, window.LogsManager.currentLevel);
                }
                break;
        }
    }

    // MARK: handleURLFragments
    // Handle URL fragments for deep linking
    static handleURLFragments() {
        if (window.location.hash) {
            const match = window.location.hash.match(/^#(.+)/);
            if (match && window.Utils?.showTab) {
                window.Utils.showTab(match[1]);
            }
        }
    }

    // MARK: setupGlobalKeyboardShortcuts
    // Set up global keyboard shortcuts
    static setupGlobalKeyboardShortcuts() {
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' && e.target.tagName !== 'TEXTAREA') {
                const form = e.target.closest('form');
                if (form && form.id !== 'tokenForm') {
                    e.preventDefault();
                    form.dispatchEvent(new Event('submit'));
                }
            }
        });
    }

    // MARK: setupAlertDismissal
    // Set up alert dismissal by clicking outside
    static setupAlertDismissal() {
        document.addEventListener('click', function(e) {
            const alert = document.getElementById('alert');
            if (alert && !alert.classList.contains('hidden') && !alert.contains(e.target)) {
                setTimeout(() => alert.classList.add('hidden'), 100);
            }
        });
    }

    // MARK: cleanup
    // Cleanup function for page unload
    static cleanup() {
        if (window.StatusManager) {
            window.StatusManager.stopStatusRefresh();
        }
        if (window.LogsManager) {
            window.LogsManager.stopLogsRefresh();
        }
    }
}

// GLOBAL SCOPE EXPORTS
window.App = App;

// MARK: addPeer
window.addPeer = function() {
    if (window.TunnelsManager) {
        window.TunnelsManager.addPeer();
    }
};

// MARK: removePeer
window.removePeer = function(index) {
    if (window.TunnelsManager) {
        window.TunnelsManager.removePeer(index);
    }
};

// APPLICATION LIFECYCLE

// MARK: DOMContentLoaded
document.addEventListener('DOMContentLoaded', function() {
    console.log('DOM loaded, initializing application...');
    App.cleanupExistingModals();
    App.handleAuthentication();
});

// MARK: cleanupExistingModals
App.cleanupExistingModals = function() {
    const existingModals = document.querySelectorAll('#tokenModal');
    existingModals.forEach(modal => modal.remove());
};

// MARK: handleAuthentication
App.handleAuthentication = function() {
    if (window.FinGuardConfig && window.FinGuardConfig.ADMIN_TOKEN) {
        // Token exists, verify and initialize
        if (window.AuthManager) {
            window.AuthManager.verifyToken(window.FinGuardConfig.ADMIN_TOKEN)
                .then(() => {
                    // After successful auth, initialize app
                    App.setupApplication();
                    App.initializeManagers();
                })
                .catch(() => {
                    // Auth failed, show token modal
                    setTimeout(() => {
                        if (window.AuthManager) {
                            window.AuthManager.showTokenModal();
                        }
                    }, 100);
                });
        }
    } else {
        // No token, show modal
        setTimeout(() => {
            if (window.AuthManager) {
                window.AuthManager.showTokenModal();
            }
        }, 100);
    }
};

// MARK: beforeunload
window.addEventListener('beforeunload', function() {
    App.cleanup();
});