// Update Management
class UpdateManager {
    static updateInfo = null;
    static updateConfig = null;

    // MARK: loadUpdateStatus
    static async loadUpdateStatus() {
        try {
            const [statusResponse, configResponse] = await Promise.all([
                window.APIClient.getUpdateStatus(),
                window.APIClient.getUpdateConfig()
            ]);
            
            UpdateManager.updateInfo = statusResponse.data;
            UpdateManager.updateConfig = configResponse.data;
            
            UpdateManager.renderUpdateStatus();
            
        } catch (error) {
            console.error('Failed to load update status:', error);
            const updateStatus = document.getElementById('updateStatus');
            if (updateStatus) {
                updateStatus.innerHTML = '<p style="color: var(--color-danger);">Failed to load update status</p>';
            }
        }
    }

    // MARK: renderUpdateStatus
    static renderUpdateStatus() {
        const updateStatus = document.getElementById('updateStatus');
        const updateControls = document.getElementById('updateControls');
        const applyBtn = document.getElementById('applyUpdateBtn');

        if (!UpdateManager.updateInfo) {
            updateStatus.innerHTML = '<p style="color: var(--color-text-secondary);">Update information not available</p>';
            return;
        }

        const info = UpdateManager.updateInfo;
        const nextCheckTime = info.next_check_time ? new Date(info.next_check_time).toLocaleString() : 'Not scheduled';
        const lastCheckTime = info.last_check_time ? new Date(info.last_check_time).toLocaleString() : 'Never';

        updateStatus.innerHTML = `
            <div class="list-item">
                <div>
                    <strong>Current Version</strong><br>
                    <small>Application version currently running</small>
                </div>
                <span style="color: var(--color-accent); font-weight: bold; font-family: monospace;">
                    ${window.Utils.escapeHtml(info.current_version)}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Latest Available</strong><br>
                    <small>Most recent version on GitHub</small>
                </div>
                <span style="color: var(--color-accent); font-weight: bold; font-family: monospace;">
                    ${window.Utils.escapeHtml(info.latest_version)}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Update Available</strong><br>
                    <small>Newer version ready for installation</small>
                </div>
                <span class="status ${info.available ? 'stopped' : 'running'}">
                    ${info.available ? 'Available' : 'Up to date'}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Auto-Updates</strong><br>
                    <small>Automatic update checking: ${window.Utils.escapeHtml(info.update_schedule || 'Not configured')}</small>
                </div>
                <span class="status ${info.auto_update_enabled ? 'running' : 'stopped'}">
                    ${info.auto_update_enabled ? 'Enabled' : 'Disabled'}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Last Check</strong><br>
                    <small>Most recent update check performed</small>
                </div>
                <span style="color: var(--color-text-secondary); font-size: 0.9rem;">
                    ${lastCheckTime}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Next Scheduled Check</strong><br>
                    <small>When the next automatic check will occur</small>
                </div>
                <span style="color: var(--color-text-secondary); font-size: 0.9rem;">
                    ${nextCheckTime}
                </span>
            </div>
        `;

        updateControls.style.display = 'flex';
        updateControls.style.gap = '1rem';
        updateControls.style.justifyContent = 'center';

        if (info.available && applyBtn) {
            applyBtn.style.display = 'inline-block';
        } else if (applyBtn) {
            applyBtn.style.display = 'none';
        }
    }

    // MARK: checkForUpdate
    static async checkForUpdate() {
        try {
            window.Utils.showAlert('Checking for updates...', 'info');
            
            const response = await window.APIClient.checkForUpdate();
            UpdateManager.updateInfo = response.data;
            
            UpdateManager.renderUpdateStatus();
            
            if (response.data.available) {
                window.Utils.showAlert(`Update available! Version ${response.data.latest_version} is ready for installation.`, 'success');
            } else {
                window.Utils.showAlert('No updates available. You are running the latest version.', 'success');
            }
            
        } catch (error) {
            console.error('Failed to check for updates:', error);
            window.Utils.showAlert(`Failed to check for updates: ${error.message}`, 'error');
        }
    }

    // MARK: applyUpdate
    static async applyUpdate() {
        if (!UpdateManager.updateInfo || !UpdateManager.updateInfo.available) {
            window.Utils.showAlert('No updates available to apply', 'error');
            return;
        }

        if (!confirm(`Apply update to version ${UpdateManager.updateInfo.latest_version}?\n\nThe application will restart automatically after the update completes.`)) {
            return;
        }

        try {
            window.Utils.showAlert('Applying update... The application will restart shortly.', 'info');
            
            await window.APIClient.applyUpdate();
            
            setTimeout(() => {
                window.Utils.showAlert('Update applied successfully! Application is restarting...', 'success');
            }, 2000);
            
            setTimeout(() => {
                if (confirm('The application should have restarted. Would you like to reload the page?')) {
                    window.location.reload();
                }
            }, 10000);
            
        } catch (error) {
            console.error('Failed to apply update:', error);
            window.Utils.showAlert(`Failed to apply update: ${error.message}`, 'error');
        }
    }

    // MARK: showUpdateConfig
    static async showUpdateConfig() {
        try {
            if (!UpdateManager.updateConfig) {
                const response = await window.APIClient.getUpdateConfig();
                UpdateManager.updateConfig = response.data;
            }

            const modal = document.getElementById('updateConfigModal');
            const form = document.getElementById('updateConfigForm');
            
            document.getElementById('updateEnabled').checked = UpdateManager.updateConfig.enabled;
            document.getElementById('updateSchedule').value = UpdateManager.updateConfig.schedule || '0 3 * * *';
            document.getElementById('autoApply').checked = UpdateManager.updateConfig.auto_apply;
            document.getElementById('backupDir').value = UpdateManager.updateConfig.backup_dir || './backups';
            
            modal.style.display = 'flex';
            
            form.onsubmit = async (e) => {
                e.preventDefault();
                await UpdateManager.saveUpdateConfig();
            };
            
        } catch (error) {
            console.error('Failed to load update config:', error);
            window.Utils.showAlert(`Failed to load update configuration: ${error.message}`, 'error');
        }
    }

    // MARK: hideUpdateConfig
    static hideUpdateConfig() {
        const modal = document.getElementById('updateConfigModal');
        modal.style.display = 'none';
    }

    // MARK: saveUpdateConfig
    static async saveUpdateConfig() {
        try {
            const config = {
                enabled: document.getElementById('updateEnabled').checked,
                schedule: document.getElementById('updateSchedule').value.trim(),
                auto_apply: document.getElementById('autoApply').checked,
                backup_dir: document.getElementById('backupDir').value.trim()
            };

            if (!config.schedule) {
                window.Utils.showAlert('Schedule is required', 'error');
                return;
            }

            if (!config.backup_dir) {
                window.Utils.showAlert('Backup directory is required', 'error');
                return;
            }

            const cronFields = config.schedule.split(/\s+/);
            if (cronFields.length !== 5) {
                window.Utils.showAlert('Invalid cron format. Use: minute hour day month weekday', 'error');
                return;
            }

            await window.APIClient.saveUpdateConfig(config);
            
            UpdateManager.updateConfig = config;
            UpdateManager.hideUpdateConfig();
            
            window.Utils.showAlert('Update configuration saved successfully!', 'success');
            
            setTimeout(() => {
                UpdateManager.loadUpdateStatus();
            }, 1000);
            
        } catch (error) {
            console.error('Failed to save update config:', error);
            window.Utils.showAlert(`Failed to save configuration: ${error.message}`, 'error');
        }
    }

    // MARK: initializeCronHelp
    static initializeCronHelp() {
        const scheduleInput = document.getElementById('updateSchedule');
        if (scheduleInput) {
            scheduleInput.title = `Common schedules:
• 0 3 * * *    - Daily at 3:00 AM
• 0 3 * * 0    - Weekly on Sunday at 3:00 AM  
• 0 3 1 * *    - Monthly on the 1st at 3:00 AM
• */30 * * * * - Every 30 minutes`;
        }
    }
}

// Export to global scope
window.UpdateManager = UpdateManager;