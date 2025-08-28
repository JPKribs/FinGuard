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
        
        // Support for different alert types
        const validTypes = ['success', 'error', 'info', 'warning'];
        const alertType = validTypes.includes(type) ? type : 'success';
        
        alert.className = `alert ${alertType}`;
        alert.textContent = message;
        alert.classList.remove('hidden');
        
        // Auto-hide after different durations based on type
        const duration = alertType === 'error' ? 8000 : alertType === 'info' ? 3000 : 5000;
        
        setTimeout(() => {
            alert.classList.add('hidden');
        }, duration);
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

    static scrollToElement(elementId, behavior = 'smooth') {
        const element = document.getElementById(elementId);
        if (element) {
            element.scrollIntoView({ behavior, block: 'start' });
        }
    }

    static scrollToTop(behavior = 'smooth') {
        window.scrollTo({
            top: 0,
            behavior
        });
    }

    static clearForm(formId) {
        const form = document.getElementById(formId);
        if (form) {
            form.reset();
            
            // Focus on first input
            const firstInput = form.querySelector('input, select, textarea');
            if (firstInput) {
                firstInput.focus();
            }
        }
    }

    static validateRequired(formId) {
        const form = document.getElementById(formId);
        if (!form) return false;
        
        const requiredFields = form.querySelectorAll('[required]');
        for (const field of requiredFields) {
            if (!field.value.trim()) {
                field.focus();
                Utils.showAlert(`${field.previousElementSibling?.textContent || 'Field'} is required`, 'error');
                return false;
            }
        }
        return true;
    }

    static debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    static formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    static formatUptime(seconds) {
        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);
        
        if (days > 0) {
            return `${days}d ${hours}h ${minutes}m`;
        } else if (hours > 0) {
            return `${hours}h ${minutes}m`;
        } else {
            return `${minutes}m`;
        }
    }

    static copyToClipboard(text) {
        if (navigator.clipboard) {
            navigator.clipboard.writeText(text).then(() => {
                Utils.showAlert('Copied to clipboard!', 'success');
            }).catch(err => {
                Utils.showAlert('Failed to copy to clipboard', 'error');
                console.error('Clipboard error:', err);
            });
        } else {
            // Fallback for older browsers
            const textArea = document.createElement('textarea');
            textArea.value = text;
            document.body.appendChild(textArea);
            textArea.select();
            try {
                document.execCommand('copy');
                Utils.showAlert('Copied to clipboard!', 'success');
            } catch (err) {
                Utils.showAlert('Failed to copy to clipboard', 'error');
            }
            document.body.removeChild(textArea);
        }
    }
}

// Export to global scope
window.Utils = Utils;