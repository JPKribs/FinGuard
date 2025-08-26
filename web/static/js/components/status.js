// System Status Management
class StatusManager {
    static statusRefreshInterval = null;

    static async loadStatus() {
        try {
            window.Utils.showLoading('systemStatus');
            
            const response = await window.APIClient.getStatus();
            const status = response.data;
            
            StatusManager.renderSystemStatus(status);
            
        } catch (error) {
            console.error('Failed to load status:', error);
            if (!error.message.includes('Authentication')) {
                document.getElementById('systemStatus').innerHTML = '<p style="color: var(--color-danger);">Failed to load system status</p>';
            }
        }
    }

    static renderSystemStatus(status) {
        const systemStatus = document.getElementById('systemStatus');
        
        systemStatus.innerHTML = `
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
            <div class="list-item">
                <div>
                    <strong>Active Services</strong><br>
                    <small>Currently configured services</small>
                </div>
                <span style="color: var(--color-text-secondary); font-weight: bold;">${status.services}</span>
            </div>
            <div class="list-item">
                <div>
                    <strong>System Health</strong><br>
                    <small>Overall system status</small>
                </div>
                <span class="status ${status.proxy && status.tunnels ? 'running' : 'stopped'}">
                    ${status.proxy && status.tunnels ? 'Healthy' : 'Degraded'}
                </span>
            </div>
        `;
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