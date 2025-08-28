// System Status Management
class StatusManager {
    static statusRefreshInterval = null;

    static async loadStatus() {
        try {
            window.Utils.showLoading('systemStatus');
            
            const response = await window.APIClient.getStatus();
            const status = response.data;
            
            StatusManager.renderSystemStatus(status);
            
            if (window.UpdateManager) {
                window.UpdateManager.loadUpdateStatus();
            }
            
        } catch (error) {
            console.error('Failed to load status:', error);
            if (!error.message.includes('Authentication')) {
                document.getElementById('systemStatus').innerHTML = '<p style="color: var(--color-danger);">Failed to load system status</p>';
            }
        }
    }

    // MARK: renderSystemStatus
    static renderSystemStatus(status) {
        const systemStatus = document.getElementById('systemStatus');

        let interfacesHtml = '';
        if (status.interfaces && status.interfaces.length > 0) {
            const activeInterfaces = status.interfaces.filter(iface => iface.is_up);
            interfacesHtml = `<small>Active interfaces: ${activeInterfaces.length}</small>`;
        }

        // Convert array of IPs into line breaks
        let systemIPsHtml = 'unavailable';
        if (Array.isArray(status.system_ip) && status.system_ip.length > 0) {
            systemIPsHtml = status.system_ip.map(ip => `<div>${ip}</div>`).join('');
        }

        systemStatus.innerHTML = `
            <div class="list-item">
                <div>
                    <strong>System Health</strong><br>
                    <small>Overall system status</small>
                </div>
                <span class="status ${status.proxy && status.tunnels ? 'running' : 'stopped'}">
                    ${status.proxy && status.tunnels ? 'Healthy' : 'Degraded'}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>IPv4 Addresses</strong><br>
                    <small>Primary network interface</small>
                    ${interfacesHtml}
                </div>
                <span style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block; text-align: right;">
                    ${systemIPsHtml}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Active Services</strong><br>
                    <small>Currently configured services</small>
                </div>
                <span style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block; text-align: right;">
                    ${String(status.services).padStart(2, '0')}
                </span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Proxy Server</strong><br>
                    <small>Handles HTTP requests and routing</small>
                </div>
                <span class="status ${status.proxy ? 'running' : 'stopped'}">${status.proxy ? 'Running' : 'Stopped'}</span>
            </div>
            <div class="list-item">
                <div>
                    <strong>Tunnel Manager</strong><br>
                    <small>Manages WireGuard connections</small>
                </div>
                <span class="status ${status.tunnels ? 'running' : 'stopped'}">${status.tunnels ? 'Running' : 'Stopped'}</span>
            </div>
        `;
    }

    // MARK: restartSystem
    static async restartSystem() {
        if (!confirm('Restart the FinGuard application?\n\nThis will temporarily disconnect all services and tunnels during the restart process.')) {
            return;
        }

        try {
            window.Utils.showAlert('Initiating system restart...', 'info');
            
            await window.APIClient.restartSystem();
            
            window.Utils.showAlert('System restart initiated! The application will be back online shortly.', 'success');
            
            // Show reconnection message after a delay
            setTimeout(() => {
                window.Utils.showAlert('Application is restarting. Please refresh the page in a few seconds.', 'info');
            }, 2000);
            
        } catch (error) {
            console.error('Failed to restart system:', error);
            window.Utils.showAlert(`Failed to restart system: ${error.message}`, 'error');
        }
    }

    // MARK: shutdownSystem
    static async shutdownSystem() {
        if (!confirm('Shutdown the FinGuard application?\n\nThis will stop all services and tunnels. You will need to manually restart the application.')) {
            return;
        }

        try {
            window.Utils.showAlert('Initiating system shutdown...', 'warning');
            
            await window.APIClient.shutdownSystem();
            
            window.Utils.showAlert('System shutdown initiated. The application will stop shortly.', 'success');
            
            // Show final message after a delay
            setTimeout(() => {
                window.Utils.showAlert('Application is shutting down. This page will become unavailable.', 'warning');
            }, 2000);
            
        } catch (error) {
            console.error('Failed to shutdown system:', error);
            window.Utils.showAlert(`Failed to shutdown system: ${error.message}`, 'error');
        }
    }

    // MARK: signOut
    static signOut() {
        if (!confirm('Sign out of FinGuard?\n\nYou will need to re-enter your admin token to access the interface.')) {
            return;
        }

        // Clear token from config and localStorage
        localStorage.removeItem('adminToken');
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = null;
        }
        
        window.Utils.showAlert('Signed out successfully. Redirecting to login...', 'success');
        
        // Stop any active refresh intervals
        StatusManager.stopStatusRefresh();
        if (window.LogsManager) {
            window.LogsManager.stopLogsRefresh();
        }
        
        // Clear the page content and show auth modal after a brief delay
        setTimeout(() => {
            // Hide all content tabs
            document.querySelectorAll('.content').forEach(content => {
                content.classList.remove('active');
            });
            
            // Show login modal
            window.AuthManager.showTokenModal();
        }, 1000);
    }

    static startStatusRefresh() {
        if (StatusManager.statusRefreshInterval) {
            clearInterval(StatusManager.statusRefreshInterval);
        }
        
        StatusManager.statusRefreshInterval = setInterval(() => {
            const statusTab = document.getElementById('status');
            if (statusTab && statusTab.classList.contains('active') && window.FinGuardConfig.ADMIN_TOKEN) {
                StatusManager.loadStatus();
            }
        }, 30000);
    }

    static stopStatusRefresh() {
        if (StatusManager.statusRefreshInterval) {
            clearInterval(StatusManager.statusRefreshInterval);
            StatusManager.statusRefreshInterval = null;
        }
    }
}

// Export to global scope
window.StatusManager = StatusManager;