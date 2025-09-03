
// SYSTEM STATUS MANAGEMENT

class StatusManager {
    static statusRefreshInterval = null;

    // STATUS LOADING

    // MARK: loadStatus
    static async loadStatus() {
        try {
            window.Utils.showLoading('systemStatus');
            
            const response = await window.APIClient.getStatus();
            this.renderSystemStatus(response.data);
            this.loadAdditionalStatus();
        } catch (error) {
            this.handleStatusError(error);
        }
    }

    // MARK: loadAdditionalStatus
    static loadAdditionalStatus() {
        if (window.UpdateManager) {
            window.UpdateManager.loadUpdateStatus();
        }
    }

    // MARK: handleStatusError
    static handleStatusError(error) {
        console.error('Failed to load status:', error);
        
        if (!error.message.includes('Authentication')) {
            const systemStatus = document.getElementById('systemStatus');
            systemStatus.innerHTML = '<p style="color: var(--color-danger);">Failed to load system status</p>';
        }
    }

    // STATUS RENDERING

    // MARK: renderSystemStatus
    static renderSystemStatus(status) {
        const systemStatus = document.getElementById('systemStatus');
        systemStatus.innerHTML = this.generateStatusHTML(status);
    }

    // MARK: generateStatusHTML
    static generateStatusHTML(status) {
        return [
            this.createSystemHealthItem(status),
            this.createIPAddressesItem(status),
            this.createActiveServicesItem(status),
            this.createJellyfinBroadcasterItem(status),
            this.createProxyServerItem(status),
            this.createTunnelManagerItem(status)
        ].join('');
    }

    // MARK: createSystemHealthItem
    static createSystemHealthItem(status) {
        const proxyHealthy = status.proxy;
        const tunnelsHealthy = status.tunnels;
        
        // Jellyfin is considered healthy if: no services OR (has services AND running)
        const jellyfinServices = status.jellyfin ? status.jellyfin.services : 0;
        const jellyfinHealthy = jellyfinServices === 0 || (status.jellyfin && status.jellyfin.running);
        
        const isHealthy = proxyHealthy && tunnelsHealthy && jellyfinHealthy;
        const healthStatus = isHealthy ? 'Healthy' : 'Degraded';
        const healthClass = isHealthy ? 'running' : 'stopped';

        return this.createStatusItem(
            'System Health',
            'Overall system status',
            healthStatus,
            `status ${healthClass}`
        );
    }

    // MARK: createIPAddressesItem
    static createIPAddressesItem(status) {
        const ipv4Ifaces = status.interfaces.map(i => ({
            addresses: i.addresses.filter(ip => ip.includes('.')),
            is_up: i.is_up
        })).filter(i => i.addresses.length > 0 || i.is_up === false);

        const ipv6Ifaces = status.interfaces.map(i => ({
            addresses: i.addresses.filter(ip => ip.includes(':')),
            is_up: i.is_up
        })).filter(i => i.addresses.length > 0 || i.is_up === false);

        const ipv4UpCount = ipv4Ifaces.filter(i => i.is_up).length;
        const ipv6UpCount = ipv6Ifaces.filter(i => i.is_up).length;

        return `
            <div class="list-item">
                <div>
                    <strong>IPv4 Addresses</strong><br>
                    <small>Active interfaces: ${ipv4UpCount}</small>
                </div>
                ${this.generateIPsHtml(ipv4Ifaces)}
            </div>
            <div class="list-item">
                <div>
                    <strong>IPv6 Addresses</strong><br>
                    <small>Active interfaces: ${ipv6UpCount}</small>
                </div>
                ${this.generateIPsHtml(ipv6Ifaces)}
            </div>
        `;
    }

    // MARK: generateIPsHtml
    static generateIPsHtml(ifaces) {
        if (!Array.isArray(ifaces) || ifaces.length === 0) {
            return `<span style="color: var(--color-secondary); font-weight: bold; font-family: monospace; display: block; text-align: right;">-</span>`;
        }

        const allIPs = ifaces.flatMap(iface => {
            if (!iface.addresses || iface.addresses.length === 0) return [];
            return iface.addresses.map(ip => {
                const color = iface.is_up ? 'var(--color-accent)' : 'var(--color-secondary)';
                return `<div style="color: ${color}; font-weight: bold; font-family: monospace; text-align: right;">${ip}</div>`;
            });
        });

        if (allIPs.length === 0) {
            return `<div style="color: var(--color-secondary); font-weight: bold; font-family: monospace; text-align: right;">-</div>`;
        }

        return `<div style="display: flex; flex-direction: column;">${allIPs.join('')}</div>`;
    }

    static createJellyfinBroadcasterItem(status) {
        const jellyfinRunning = status.jellyfin && status.jellyfin.running;
        const jellyfinServices = status.jellyfin ? status.jellyfin.services : 0;
        
        let statusText, statusClass, description;
        
        if (jellyfinServices === 0) {
            statusText = 'Inactive';
            statusClass = 'status stopped';
            description = 'No Jellyfin services configured';
        } else if (jellyfinRunning) {
            statusText = `Broadcasting (${jellyfinServices})`;
            statusClass = 'status running';
            description = `Broadcasting ${jellyfinServices} Jellyfin service${jellyfinServices > 1 ? 's' : ''}`;
        } else {
            statusText = 'Stopped';
            statusClass = 'status stopped';
            description = `${jellyfinServices} Jellyfin service${jellyfinServices > 1 ? 's' : ''} configured but not broadcasting`;
        }

        return this.createStatusItem(
            'Jellyfin Discovery',
            description,
            statusText,
            statusClass
        );
    }

    // MARK: generateInterfacesHtml
    static generateInterfacesHtml(interfaces) {
        if (!interfaces || interfaces.length === 0) return '';
        
        const activeInterfaces = interfaces.filter(iface => iface.is_up);
        return `<small>Active interfaces: ${activeInterfaces.length}</small>`;
    }

    // MARK: generateSystemIPsHtml
    static generateSystemIPsHtml(systemIPs) {
        if (!Array.isArray(systemIPs) || systemIPs.length === 0) {
            return 'unavailable';
        }
        
        return systemIPs.map(ip => `<div>${ip}</div>`).join('');
    }

    // MARK: createActiveServicesItem
    static createActiveServicesItem(status) {
        const serviceCount = String(status.services).padStart(2, '0');
        
        return `
            <div class="list-item">
                <div>
                    <strong>Active Services</strong><br>
                    <small>Currently configured services</small>
                </div>
                <span style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block; text-align: right;">
                    ${serviceCount}
                </span>
            </div>
        `;
    }

    // MARK: createProxyServerItem
    static createProxyServerItem(status) {
        return this.createStatusItem(
            'Proxy Server',
            'Handles HTTP requests and routing',
            status.proxy ? 'Running' : 'Stopped',
            `status ${status.proxy ? 'running' : 'stopped'}`
        );
    }

    // MARK: createTunnelManagerItem
    static createTunnelManagerItem(status) {
        return this.createStatusItem(
            'Tunnel Manager',
            'Manages WireGuard connections',
            status.tunnels ? 'Running' : 'Stopped',
            `status ${status.tunnels ? 'running' : 'stopped'}`
        );
    }

    // MARK: createStatusItem
    static createStatusItem(title, description, value, className) {
        return `
            <div class="list-item">
                <div>
                    <strong>${title}</strong><br>
                    <small>${description}</small>
                </div>
                <span class="${className}">${value}</span>
            </div>
        `;
    }

    // SYSTEM CONTROL ACTIONS

    // MARK: restartSystem
    static async restartSystem() {
        if (!this.confirmRestart()) return;

        try {
            await this.performRestart();
            this.showRestartMessages();
        } catch (error) {
            this.handleSystemActionError(error, 'restart');
        }
    }

    // MARK: confirmRestart
    static confirmRestart() {
        return confirm('Restart the FinGuard application?\n\nThis will temporarily disconnect all services and tunnels during the restart process.');
    }

    // MARK: performRestart
    static async performRestart() {
        window.Utils.showAlert('Initiating system restart...', 'info');
        await window.APIClient.restartSystem();
    }

    // MARK: showRestartMessages
    static showRestartMessages() {
        window.Utils.showAlert('System restart initiated! The application will be back online shortly.', 'success');
        
        setTimeout(() => {
            window.Utils.showAlert('Application is restarting. Please refresh the page in a few seconds.', 'info');
        }, 2000);
    }

    // MARK: shutdownSystem
    static async shutdownSystem() {
        if (!this.confirmShutdown()) return;

        try {
            await this.performShutdown();
            this.showShutdownMessages();
        } catch (error) {
            this.handleSystemActionError(error, 'shutdown');
        }
    }

    // MARK: confirmShutdown
    static confirmShutdown() {
        return confirm('Shutdown the FinGuard application?\n\nThis will stop all services and tunnels. You will need to manually restart the application.');
    }

    // MARK: performShutdown
    static async performShutdown() {
        window.Utils.showAlert('Initiating system shutdown...', 'warning');
        await window.APIClient.shutdownSystem();
    }

    // MARK: showShutdownMessages
    static showShutdownMessages() {
        window.Utils.showAlert('System shutdown initiated. The application will stop shortly.', 'success');
        
        setTimeout(() => {
            window.Utils.showAlert('Application is shutting down. This page will become unavailable.', 'warning');
        }, 2000);
    }

    // MARK: handleSystemActionError
    static handleSystemActionError(error, action) {
        console.error(`Failed to ${action} system:`, error);
        window.Utils.showAlert(`Failed to ${action} system: ${error.message}`, 'error');
    }

    // AUTHENTICATION MANAGEMENT

    // MARK: signOut
    static signOut() {
        if (!this.confirmSignOut()) return;

        this.clearAuthenticationData();
        this.stopAllRefreshIntervals();
        this.showSignOutMessage();
        this.redirectToLogin();
    }

    // MARK: confirmSignOut
    static confirmSignOut() {
        return confirm('Sign out of FinGuard?\n\nYou will need to re-enter your admin token to access the interface.');
    }

    // MARK: clearAuthenticationData
    static clearAuthenticationData() {
        localStorage.removeItem('adminToken');
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = null;
        }
    }

    // MARK: stopAllRefreshIntervals
    static stopAllRefreshIntervals() {
        this.stopStatusRefresh();
        if (window.LogsManager) {
            window.LogsManager.stopLogsRefresh();
        }
    }

    // MARK: showSignOutMessage
    static showSignOutMessage() {
        window.Utils.showAlert('Signed out successfully. Redirecting to login...', 'success');
    }

    // MARK: redirectToLogin
    static redirectToLogin() {
        setTimeout(() => {
            this.hideAllContent();
            window.AuthManager.showTokenModal();
        }, 1000);
    }

    // MARK: hideAllContent
    static hideAllContent() {
        document.querySelectorAll('.content').forEach(content => {
            content.classList.remove('active');
        });
    }

    // AUTO-REFRESH MANAGEMENT

    // MARK: startStatusRefresh
    static startStatusRefresh() {
        this.stopStatusRefresh();
        
        this.statusRefreshInterval = setInterval(() => {
            this.refreshStatusIfActive();
        }, 30000);
    }

    // MARK: refreshStatusIfActive
    static refreshStatusIfActive() {
        const statusTab = document.getElementById('status');
        const isActive = statusTab && statusTab.classList.contains('active');
        const hasToken = window.FinGuardConfig.ADMIN_TOKEN;
        
        if (isActive && hasToken) {
            this.loadStatus();
        }
    }

    // MARK: stopStatusRefresh
    static stopStatusRefresh() {
        if (this.statusRefreshInterval) {
            clearInterval(this.statusRefreshInterval);
            this.statusRefreshInterval = null;
        }
    }

    // MARK: initialize
    static initialize() {
        this.startStatusRefresh();
        // Load status immediately
        this.loadStatus();
    }
}

// GLOBAL SCOPE EXPORT
window.StatusManager = StatusManager;