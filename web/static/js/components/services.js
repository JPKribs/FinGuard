// Services Management
class ServicesManager {
    static async loadServices() {
        try {
            window.Utils.showLoading('servicesList');
            
            const [servicesResponse, tunnelsResponse] = await Promise.all([
                window.APIClient.getServices(),
                window.APIClient.getTunnels()
            ]);
            
            const services = servicesResponse.data || [];
            const tunnels = tunnelsResponse.data || [];
            
            ServicesManager.renderServicesList(services);
            ServicesManager.updateTunnelsDropdown(tunnels);
            
        } catch (error) {
            console.error('Failed to load services:', error);
            if (!error.message.includes('Authentication')) {
                document.getElementById('servicesList').innerHTML = '<p style="color: var(--color-danger);">Failed to load services</p>';
            }
        }
    }

    static renderServicesList(services) {
        const servicesList = document.getElementById('servicesList');
        
        if (services.length === 0) {
            servicesList.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No services configured</p>';
            return;
        }

        servicesList.innerHTML = services.map(service => {
            const capabilities = [];
            if (service.websocket) capabilities.push('✓ WebSocket');
            if (service.default) capabilities.push('✓ Default');
            if (service.publish_mdns) capabilities.push('✓ mDNS');
            
            return `
                <div class="list-item">
                    <div class="service-info">
                        <strong>${window.Utils.escapeHtml(service.name)}.local</strong>
                        <small>${window.Utils.escapeHtml(service.upstream)}</small>
                        ${service.tunnel ? `<small>Tunnel: ${window.Utils.escapeHtml(service.tunnel)}</small>` : ''}
                        ${capabilities.length > 0 ? `<br><small>${capabilities.join(' | ')}</small>` : ''}
                    </div>
                    <div class="actions">
                        <span class="status ${service.status === 'running' ? 'running' : 'stopped'}">${service.status}</span>
                        <button class="btn-danger" onclick="window.ServicesManager.deleteService('${window.Utils.escapeHtml(service.name)}')">Delete</button>
                    </div>
                </div>
            `;
        }).join('');
    }

    static updateTunnelsDropdown(tunnels) {
        const tunnelSelect = document.getElementById('serviceTunnel');
        tunnelSelect.innerHTML = '<option value="">None</option>' + 
            tunnels.map(tunnel => `<option value="${window.Utils.escapeHtml(tunnel.name)}">${window.Utils.escapeHtml(tunnel.name)}</option>`).join('');
    }

    static async deleteService(name) {
        if (!confirm(`Delete service "${name}"?`)) return;
        
        try {
            await window.APIClient.deleteService(name);
            window.Utils.showAlert(`Service "${name}" deleted successfully`);
            ServicesManager.loadServices();
        } catch (error) {
            console.error('Failed to delete service:', error);
        }
    }

    static initializeForm() {
        document.getElementById('serviceForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const service = {
                name: document.getElementById('serviceName').value.trim(),
                upstream: document.getElementById('serviceUpstream').value.trim(),
                websocket: document.getElementById('serviceWebsocket').checked,
                default: document.getElementById('serviceDefault').checked,
                publish_mdns: document.getElementById('serviceMDNS').checked,
                tunnel: document.getElementById('serviceTunnel').value || undefined
            };
            
            if (!service.name || !service.upstream) {
                window.Utils.showAlert('Service name and upstream URL are required', 'error');
                return;
            }
            
            if (!/^[a-zA-Z0-9-]+$/.test(service.name)) {
                window.Utils.showAlert('Service name can only contain letters, numbers, and hyphens', 'error');
                return;
            }
            
            try {
                await window.APIClient.addService(service);
                window.Utils.showAlert(`Service "${service.name}" added successfully`);
                document.getElementById('serviceForm').reset();
                ServicesManager.loadServices();
            } catch (error) {
                console.error('Failed to add service:', error);
            }
        });
    }
}

// Export to global scope
window.ServicesManager = ServicesManager;