// Utility Functions
class Utils {
    static showTab(tabName) {
        document.querySelectorAll('.tab').forEach(tab => tab.classList.remove('active'));
        document.querySelectorAll('.content').forEach(content => content.classList.remove('active'));
        
        document.querySelector(`[onclick*="${tabName}"]`).classList.add('active');
        document.getElementById(tabName).classList.add('active');
        
        switch(tabName) {
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
            case 'status':
                if (window.StatusManager) {
                    window.StatusManager.loadStatus();
                }
                break;
            case 'logs':
                if (window.LogsManager) {
                    window.LogsManager.loadLogs(window.LogsManager.currentPage, window.LogsManager.currentLevel);
                    window.LogsManager.startLogsRefresh();
                }
                break;
        }

        // Stop other refreshes when switching tabs
        if (tabName !== 'status' && window.StatusManager) {
            window.StatusManager.stopStatusRefresh();
        }
        if (tabName !== 'logs' && window.LogsManager) {
            window.LogsManager.stopLogsRefresh();
        }
    }

    static showAlert(message, type = 'success') {
        const alert = document.getElementById('alert');
        alert.className = `alert ${type}`;
        alert.textContent = message;
        alert.classList.remove('hidden');
        
        setTimeout(() => {
            alert.classList.add('hidden');
        }, 5000);
    }

    static showLoading(elementId) {
        document.getElementById(elementId).innerHTML = '<div class="loading"></div>';
    }

    static escapeHtml(text) {
        const map = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        };
        return text.replace(/[&<>"']/g, function(m) { return map[m]; });
    }
}

// Export to global scope
window.Utils = Utils;