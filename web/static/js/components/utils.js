
// UTILITY FUNCTIONS

class Utils {
    // TAB NAVIGATION

    // MARK: showTab
    static showTab(tabName) {
        this.clearActiveStates();
        this.setActiveTab(tabName);
        this.loadTabContent(tabName);
        this.manageRefreshIntervals(tabName);
    }

    // MARK: clearActiveStates
    static clearActiveStates() {
        document.querySelectorAll('.tab').forEach(tab => tab.classList.remove('active'));
        document.querySelectorAll('.content').forEach(content => content.classList.remove('active'));
    }

    // MARK: setActiveTab
    static setActiveTab(tabName) {
        const tab = document.querySelector(`[onclick*="${tabName}"]`);
        const content = document.getElementById(tabName);
        
        if (tab) tab.classList.add('active');
        if (content) content.classList.add('active');
    }

    // MARK: loadTabContent
    static loadTabContent(tabName) {
        const tabActions = {
            services: () => window.ServicesManager?.loadServices(),
            tunnels: () => window.TunnelsManager?.loadTunnels(),
            status: () => window.StatusManager?.loadStatus(),
            logs: () => {
                window.LogsManager?.loadLogs(window.LogsManager.currentPage, window.LogsManager.currentLevel);
                window.LogsManager?.startLogsRefresh();
            }
        };

        const action = tabActions[tabName];
        if (action) action();
    }

    // MARK: manageRefreshIntervals
    static manageRefreshIntervals(tabName) {
        if (tabName !== 'status' && window.StatusManager) {
            window.StatusManager.stopStatusRefresh();
        }
        if (tabName !== 'logs' && window.LogsManager) {
            window.LogsManager.stopLogsRefresh();
        }
    }

    // ALERT MANAGEMENT

    // MARK: showAlert
    static showAlert(message, type = 'success') {
        const alert = document.getElementById('alert');
        const alertType = this.validateAlertType(type);
        
        this.displayAlert(alert, message, alertType);
        this.scheduleAlertHide(alert, alertType);
    }

    // MARK: validateAlertType
    static validateAlertType(type) {
        const validTypes = ['success', 'error', 'info', 'warning'];
        return validTypes.includes(type) ? type : 'success';
    }

    // MARK: displayAlert
    static displayAlert(alert, message, type) {
        alert.className = `alert ${type}`;
        alert.textContent = message;
        alert.classList.remove('hidden');
    }

    // MARK: scheduleAlertHide
    static scheduleAlertHide(alert, type) {
        const durations = {
            error: 8000,
            info: 3000,
            warning: 5000,
            success: 5000
        };

        setTimeout(() => {
            alert.classList.add('hidden');
        }, durations[type]);
    }

    // UI HELPERS

    // MARK: showLoading
    static showLoading(elementId) {
        const element = document.getElementById(elementId);
        if (element) {
            element.innerHTML = '<div class="loading"></div>';
        }
    }

    // MARK: escapeHtml
    static escapeHtml(text) {
        const escapeMap = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        };
        return text.replace(/[&<>"']/g, char => escapeMap[char]);
    }

    // SCROLL UTILITIES

    // MARK: scrollToElement
    static scrollToElement(elementId, behavior = 'smooth') {
        const element = document.getElementById(elementId);
        if (element) {
            element.scrollIntoView({ behavior, block: 'start' });
        }
    }

    // MARK: scrollToTop
    static scrollToTop(behavior = 'smooth') {
        window.scrollTo({
            top: 0,
            behavior
        });
    }

    // FORM UTILITIES

    // MARK: clearForm
    static clearForm(formId) {
        const form = document.getElementById(formId);
        if (!form) return;

        form.reset();
        this.focusFirstInput(form);
    }

    // MARK: focusFirstInput
    static focusFirstInput(form) {
        const firstInput = form.querySelector('input, select, textarea');
        if (firstInput) {
            firstInput.focus();
        }
    }

    // MARK: validateRequired
    static validateRequired(formId) {
        const form = document.getElementById(formId);
        if (!form) return false;

        const requiredFields = form.querySelectorAll('[required]');
        
        for (const field of requiredFields) {
            if (!field.value.trim()) {
                this.showFieldError(field);
                return false;
            }
        }
        return true;
    }

    // MARK: showFieldError
    static showFieldError(field) {
        field.focus();
        const fieldName = field.previousElementSibling?.textContent || 'Field';
        this.showAlert(`${fieldName} is required`, 'error');
    }

    // PERFORMANCE UTILITIES

    // MARK: debounce
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

    // FORMATTING UTILITIES

    // MARK: formatBytes
    static formatBytes(bytes) {
        if (bytes === 0) return '0 B';
        
        const k = 1024;
        const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        
        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    // MARK: formatUptime
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

    // CLIPBOARD UTILITIES

    // MARK: copyToClipboard
    static async copyToClipboard(text) {
        try {
            await this.tryModernClipboard(text);
        } catch (error) {
            this.tryLegacyClipboard(text);
        }
    }

    // MARK: tryModernClipboard
    static async tryModernClipboard(text) {
        if (!navigator.clipboard) {
            throw new Error('Clipboard API not available');
        }

        await navigator.clipboard.writeText(text);
        this.showAlert('Copied to clipboard!', 'success');
    }

    // MARK: tryLegacyClipboard
    static tryLegacyClipboard(text) {
        const textArea = document.createElement('textarea');
        textArea.value = text;
        document.body.appendChild(textArea);
        textArea.select();

        try {
            document.execCommand('copy');
            this.showAlert('Copied to clipboard!', 'success');
        } catch (err) {
            this.showAlert('Failed to copy to clipboard', 'error');
        } finally {
            document.body.removeChild(textArea);
        }
    }
}

// GLOBAL SCOPE EXPORT

window.Utils = Utils;