// UPDATE MANAGEMENT

class UpdateManager {
    static updateInfo = null;
    static updateConfig = null;

    // STATUS LOADING

    // MARK: loadUpdateStatus
    static async loadUpdateStatus() {
        try {
            const responses = await this.fetchUpdateData();
            this.updateState(responses);
            this.renderUpdateStatus();
        } catch (error) {
            this.handleLoadError(error);
        }
    }

    // MARK: fetchUpdateData
    static async fetchUpdateData() {
        return await Promise.all([
            window.APIClient.getUpdateStatus(),
            window.APIClient.getUpdateConfig()
        ]);
    }

    // MARK: updateState
    static updateState([statusResponse, configResponse]) {
        this.updateInfo = statusResponse.data;
        this.updateConfig = configResponse.data;
    }

    // MARK: handleLoadError
    static handleLoadError(error) {
        console.error('Failed to load update status:', error);
        const updateStatus = document.getElementById('updateStatus');
        if (updateStatus) {
            updateStatus.innerHTML = '<p style="color: var(--color-danger);">Failed to load update status</p>';
        }
    }

    // STATUS RENDERING

    // MARK: renderUpdateStatus
    static renderUpdateStatus() {
        const updateStatus = document.getElementById('updateStatus');
        const updateControls = document.getElementById('updateControls');
        const applyBtn = document.getElementById('applyUpdateBtn');

        if (!this.updateInfo) {
            this.renderUnavailableStatus(updateStatus);
            return;
        }

        updateStatus.innerHTML = this.generateStatusHTML();
        this.configureControls(updateControls, applyBtn);
    }

    // MARK: renderUnavailableStatus
    static renderUnavailableStatus(container) {
        container.innerHTML = '<p style="color: var(--color-text-secondary);">Update information not available</p>';
    }

    // MARK: generateStatusHTML
    static generateStatusHTML() {
        const info = this.updateInfo;
        const nextCheckTime = info.next_check_time ? new Date(info.next_check_time).toLocaleString() : 'Not scheduled';
        const lastCheckTime = info.last_check_time ? new Date(info.last_check_time).toLocaleString() : 'Never';

        return [
            this.createStatusItem('Current Version', 'Application version currently running', info.current_version, 'text'),
            this.createStatusItem('Latest Available', 'Most recent version on GitHub', info.latest_version, 'text'),
            this.createStatusItem('Update Available', 'Newer version ready for installation', info.available ? 'Available' : 'Up to date', 'status', !info.available),
            this.createStatusItem('Auto-Updates', `Automatic update checking: ${info.update_schedule || 'Not configured'}`, info.auto_update_enabled ? 'Enabled' : 'Disabled', 'status', info.auto_update_enabled),
            this.createStatusItem('Last Check', 'Most recent update check performed', lastCheckTime, 'text'),
            this.createStatusItem('Next Scheduled Check', 'When the next automatic check will occur', nextCheckTime, 'text')
        ].join('');
    }

    // MARK: createStatusItem
    static createStatusItem(title, description, value, type, isPositive = false) {
        const escapedValue = window.Utils.escapeHtml(String(value));
        
        let valueHtml;
        if (type === 'status') {
            const valueClass = this.getValueClass(type, isPositive);
            valueHtml = `<span class="${valueClass}">${escapedValue}</span>`;
        } else {
            valueHtml = `<span style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block; text-align: right;">${escapedValue}</span>`;
        }

        return `
            <div class="list-item">
                <div>
                    <strong>${title}</strong><br>
                    <small>${description}</small>
                </div>
                ${valueHtml}
            </div>
        `;
    }

    // MARK: getValueClass
    static getValueClass(type, isPositive) {
        if (type === 'version') return 'color: var(--color-accent); font-weight: bold; font-family: monospace;';
        if (type === 'status') return `status ${isPositive ? 'running' : 'stopped'}`;
        if (type === 'time') return 'color: var(--color-text-secondary); font-size: 0.9rem;';
        return '';
    }

    // MARK: configureControls
    static configureControls(updateControls, applyBtn) {
        updateControls.style.display = 'flex';
        updateControls.style.gap = '1rem';
        updateControls.style.justifyContent = 'center';

        if (this.updateInfo.available && applyBtn) {
            applyBtn.style.display = 'inline-block';
        } else if (applyBtn) {
            applyBtn.style.display = 'none';
        }
    }

    // UPDATE OPERATIONS

    // MARK: checkForUpdate
    static async checkForUpdate() {
        try {
            window.Utils.showAlert('Checking for updates...', 'info');
            
            const response = await window.APIClient.checkForUpdate();
            this.updateInfo = response.data;
            this.renderUpdateStatus();
            
            this.showUpdateCheckResult(response.data);
        } catch (error) {
            this.handleUpdateError(error, 'check for updates');
        }
    }

    // MARK: showUpdateCheckResult
    static showUpdateCheckResult(data) {
        if (data.available) {
            window.Utils.showAlert(`Update available! Version ${data.latest_version} is ready for installation.`, 'success');
        } else {
            window.Utils.showAlert('No updates available. You are running the latest version.', 'success');
        }
    }

    // MARK: applyUpdate
    static async applyUpdate() {
        if (!this.validateUpdateAvailable()) return;
        if (!this.confirmUpdateApplication()) return;

        try {
            await this.performUpdate();
            this.handleUpdateSuccess();
        } catch (error) {
            this.handleUpdateError(error, 'apply update');
        }
    }

    // MARK: validateUpdateAvailable
    static validateUpdateAvailable() {
        if (!this.updateInfo || !this.updateInfo.available) {
            window.Utils.showAlert('No updates available to apply', 'error');
            return false;
        }
        return true;
    }

    // MARK: confirmUpdateApplication
    static confirmUpdateApplication() {
        return confirm(`Apply update to version ${this.updateInfo.latest_version}?\n\nThe application will restart automatically after the update completes.`);
    }

    // MARK: performUpdate
    static async performUpdate() {
        window.Utils.showAlert('Applying update... The application will restart shortly.', 'info');
        await window.APIClient.applyUpdate();
    }

    // MARK: handleUpdateSuccess
    static handleUpdateSuccess() {
        setTimeout(() => {
            window.Utils.showAlert('Update applied successfully! Application is restarting...', 'success');
        }, 2000);
        
        setTimeout(() => {
            if (confirm('The application should have restarted. Would you like to reload the page?')) {
                window.location.reload();
            }
        }, 10000);
    }

    // MARK: handleUpdateError
    static handleUpdateError(error, operation) {
        console.error(`Failed to ${operation}:`, error);
        window.Utils.showAlert(`Failed to ${operation}: ${error.message}`, 'error');
    }

    // CONFIGURATION MANAGEMENT

    // MARK: showUpdateConfig
    static async showUpdateConfig() {
        try {
            await this.ensureConfigLoaded();
            this.displayConfigModal();
            this.setupConfigForm();
        } catch (error) {
            this.handleConfigError(error, 'load');
        }
    }

    // MARK: ensureConfigLoaded
    static async ensureConfigLoaded() {
        if (!this.updateConfig) {
            const response = await window.APIClient.getUpdateConfig();
            this.updateConfig = response.data;
        }
    }

    // MARK: displayConfigModal
    static displayConfigModal() {
        const modal = document.getElementById('updateConfigModal');
        
        this.populateConfigForm();
        modal.style.display = 'flex';
    }

    // MARK: populateConfigForm
    static populateConfigForm() {
        document.getElementById('updateEnabled').checked = this.updateConfig.enabled;
        document.getElementById('updateSchedule').value = this.updateConfig.schedule || '0 3 * * *';
        document.getElementById('autoApply').checked = this.updateConfig.auto_apply;
        document.getElementById('backupDir').value = this.updateConfig.backup_dir || './backups';
    }

    // MARK: setupConfigForm
    static setupConfigForm() {
        const form = document.getElementById('updateConfigForm');
        form.onsubmit = async (e) => {
            e.preventDefault();
            await this.saveUpdateConfig();
        };
    }

    // MARK: hideUpdateConfig
    static hideUpdateConfig() {
        const modal = document.getElementById('updateConfigModal');
        modal.style.display = 'none';
    }

    // MARK: saveUpdateConfig
    static async saveUpdateConfig() {
        try {
            const config = this.collectConfigData();
            
            if (!this.validateConfig(config)) return;
            
            await this.persistConfig(config);
            this.handleConfigSaveSuccess();
        } catch (error) {
            this.handleConfigError(error, 'save');
        }
    }

    // MARK: collectConfigData
    static collectConfigData() {
        return {
            enabled: document.getElementById('updateEnabled').checked,
            schedule: document.getElementById('updateSchedule').value.trim(),
            auto_apply: document.getElementById('autoApply').checked,
            backup_dir: document.getElementById('backupDir').value.trim()
        };
    }

    // MARK: validateConfig
    static validateConfig(config) {
        if (!config.schedule) {
            window.Utils.showAlert('Schedule is required', 'error');
            return false;
        }

        if (!config.backup_dir) {
            window.Utils.showAlert('Backup directory is required', 'error');
            return false;
        }

        if (!this.validateCronFormat(config.schedule)) {
            window.Utils.showAlert('Invalid cron format. Use: minute hour day month weekday', 'error');
            return false;
        }

        return true;
    }

    // MARK: validateCronFormat
    static validateCronFormat(schedule) {
        const cronFields = schedule.split(/\s+/);
        return cronFields.length === 5;
    }

    // MARK: persistConfig
    static async persistConfig(config) {
        await window.APIClient.saveUpdateConfig(config);
        this.updateConfig = config;
    }

    // MARK: handleConfigSaveSuccess
    static handleConfigSaveSuccess() {
        this.hideUpdateConfig();
        window.Utils.showAlert('Update configuration saved successfully!', 'success');
        
        setTimeout(() => {
            this.loadUpdateStatus();
        }, 1000);
    }

    // MARK: handleConfigError
    static handleConfigError(error, operation) {
        console.error(`Failed to ${operation} update config:`, error);
        window.Utils.showAlert(`Failed to ${operation} configuration: ${error.message}`, 'error');
    }

    // INITIALIZATION

    // MARK: initializeCronHelp
    static initializeCronHelp() {
        const scheduleInput = document.getElementById('updateSchedule');
        if (scheduleInput) {
            scheduleInput.title = this.getCronHelpText();
        }
    }

    // MARK: getCronHelpText
    static getCronHelpText() {
        return `Common schedules:
• 0 3 * * *    - Daily at 3:00 AM
• 0 3 * * 0    - Weekly on Sunday at 3:00 AM  
• 0 3 1 * *    - Monthly on the 1st at 3:00 AM
• */30 * * * * - Every 30 minutes`;
    }

    // MARK: initialize
    static initialize() {
        this.initializeCronHelp();
        this.loadUpdateStatus();
    }
}

// GLOBAL SCOPE EXPORT
window.UpdateManager = UpdateManager;