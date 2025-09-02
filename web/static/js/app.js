// Main Application Controller
class App {
    static initializeApp() {
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

        // Setup tab navigation
        document.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', function() {
                const onclick = this.getAttribute('onclick');
                if (onclick) {
                    const match = onclick.match(/showTab\('(.+?)'\)/);
                    if (match) {
                        window.Utils.showTab(match[1]);
                    }
                }
            });
        });
        
        // Global keyboard shortcuts
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' && e.target.tagName !== 'TEXTAREA') {
                const form = e.target.closest('form');
                if (form && form.id !== 'tokenForm') {
                    e.preventDefault();
                    form.dispatchEvent(new Event('submit'));
                }
            }
        });
        
        // Alert dismissal
        document.addEventListener('click', function(e) {
            const alert = document.getElementById('alert');
            if (!alert.classList.contains('hidden') && !alert.contains(e.target)) {
                setTimeout(() => alert.classList.add('hidden'), 100);
            }
        });
    }
}

// Export to global scope
window.App = App;

// Global functions for onclick handlers
window.addPeer = function() {
    window.TunnelsManager.addPeer();
};

window.removePeer = function(index) {
    window.TunnelsManager.removePeer(index);
};

// Application initialization
document.addEventListener('DOMContentLoaded', function() {
    const existingModals = document.querySelectorAll('#tokenModal');
    existingModals.forEach(modal => modal.remove());
    
    if (window.FinGuardConfig && window.FinGuardConfig.ADMIN_TOKEN) {
        window.AuthManager.verifyToken(window.FinGuardConfig.ADMIN_TOKEN);
    } else {
        setTimeout(() => {
            window.AuthManager.showTokenModal();
        }, 100);
    }
});

// Cleanup on page unload
window.addEventListener('beforeunload', function() {
    if (window.StatusManager) {
        window.StatusManager.stopStatusRefresh();
    }
});