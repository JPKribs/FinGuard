
// MAIN APPLICATION CONTROLLER

class App {
    // MARK: initializeApp
    static initializeApp() {
        this.initializeManagers();
        this.setupEventListeners();
        this.setupGlobalKeyboardShortcuts();
        this.setupAlertDismissal();
    }

    // MARK: initializeManagers
    static initializeManagers() {
        window.ServicesManager.initializeForm();
        window.TunnelsManager.initializeForm();
        window.StatusManager.startStatusRefresh();
        window.StatusManager.loadStatus();
        window.LogsManager.initializeFilter();
        window.LogsManager.loadLogs();
        window.LogsManager.startLogsRefresh();

        if (window.UpdateManager) {
            window.UpdateManager.initializeCronHelp();
        }
    }

    // MARK: setupEventListeners
    static setupEventListeners() {
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', this.handleTabClick.bind(this));
        });
    }

    // MARK: handleTabClick
    static handleTabClick(event) {
        const tab = event.currentTarget;
        const onclick = tab.getAttribute('onclick');
        
        if (onclick) {
            const match = onclick.match(/showTab\('(.+?)'\)/);
            if (match) {
                window.Utils.showTab(match[1]);
            }
        }
    }

    // MARK: setupGlobalKeyboardShortcuts
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
    static setupAlertDismissal() {
        document.addEventListener('click', function(e) {
            const alert = document.getElementById('alert');
            if (!alert.classList.contains('hidden') && !alert.contains(e.target)) {
                setTimeout(() => alert.classList.add('hidden'), 100);
            }
        });
    }

    // MARK: cleanup
    static cleanup() {
        if (window.StatusManager) {
            window.StatusManager.stopStatusRefresh();
        }
    }
}

// GLOBAL SCOPE EXPORTS

window.App = App;

// MARK: addPeer
window.addPeer = function() {
    window.TunnelsManager.addPeer();
};

// MARK: removePeer
window.removePeer = function(index) {
    window.TunnelsManager.removePeer(index);
};

// APPLICATION LIFECYCLE

// MARK: DOMContentLoaded
document.addEventListener('DOMContentLoaded', function() {
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
        window.AuthManager.verifyToken(window.FinGuardConfig.ADMIN_TOKEN);
    } else {
        setTimeout(() => {
            window.AuthManager.showTokenModal();
        }, 100);
    }
};

// MARK: beforeunload
window.addEventListener('beforeunload', function() {
    App.cleanup();
});