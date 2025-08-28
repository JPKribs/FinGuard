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
            if (service.websocket) capabilities.push('WebSocket');
            if (service.default) capabilities.push('Default');
            if (service.publish_mdns) capabilities.push('mDNS');

            const serviceIP = ServicesManager.extractIPFromURL(service.upstream);

            const infoRows = [
                { label: 'Upstream', value: service.upstream },
                ...(service.tunnel ? [{ label: 'Tunnel', value: service.tunnel }] : []),
                { label: 'WebSocket', value: service.websocket ? '✓' : '✗' },
                { label: 'Default', value: service.default ? '✓' : '✗' },
                { label: 'mDNS', value: service.publish_mdns ? '✓' : '✗' }
            ];

            return `
                <div class="list-item" style="display: flex; position: relative;">

                    <!-- Left column -->
                    <div style="flex: 1; display: flex; flex-direction: column;">
                        <strong>${window.Utils.escapeHtml(service.name)}.local</strong>
                        ${infoRows.map(row => `
                            <div style="display: flex; justify-content: space-between;">
                                <small style="color: var(--color-text-secondary);">${row.label}:</small>
                                <small style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block;">
                                    ${window.Utils.escapeHtml(String(row.value))}
                                </small>
                            </div>
                        `).join('')}
                    </div>

                    <!-- Right column -->
                    <div style="display: flex; flex-direction: column; justify-content: space-between; align-items: flex-end; margin-left: 1rem; padding-left: 1rem; border-left: 1px solid var(--color-border); align-self: stretch;">
                        <span class="status ${service.status === 'running' ? 'running' : 'stopped'}">${service.status}</span>
                        <button class="btn-danger btn-small" onclick="window.ServicesManager.deleteService('${window.Utils.escapeHtml(service.name)}')">Delete</button>                   
                    </div>
                </div>
            `;
        }).join('');
    }

    static extractIPFromURL(url) {
        try {
            const urlObj = new URL(url);
            const hostname = urlObj.hostname;
            
            // Check if hostname is already an IP address
            const ipRegex = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
            if (ipRegex.test(hostname)) {
                return hostname;
            }
            
            return hostname;
        } catch (e) {
            return null;
        }
    }

    static updateTunnelsDropdown(tunnels) {
        const tunnelSelect = document.getElementById('serviceTunnel');
        tunnelSelect.innerHTML = '<option value="">None</option>' + 
            tunnels.map(tunnel => 
                `<option value="${window.Utils.escapeHtml(tunnel.name)}" title="Service IP will be added as route to this tunnel">
                    ${window.Utils.escapeHtml(tunnel.name)}
                </option>`
            ).join('');
    }

    static async deleteService(name) {
        if (!confirm(`Delete service "${name}"?\n\nThis will also remove any associated routes from tunnels.`)) return;
        
        try {
            await window.APIClient.deleteService(name);
            window.Utils.showAlert(`Service "${name}" deleted successfully (routes removed)`, 'success');
            ServicesManager.loadServices();
            
            // Reload tunnels to show updated routes
            if (window.TunnelsManager) {
                window.TunnelsManager.loadTunnels();
            }
        } catch (error) {
            console.error('Failed to delete service:', error);
            window.Utils.showAlert(`Failed to delete service "${name}": ${error.message}`, 'error');
        }
    }

    // MARK: initializeForm
    static initializeForm() {
        const form = document.getElementById('serviceForm');
        if (!form) {
            console.error('Service form not found');
            return;
        }

        // Add help text for tunnel selection
        const tunnelSelect = document.getElementById('serviceTunnel');
        if (tunnelSelect) {
            tunnelSelect.addEventListener('change', function() {
                const helpText = document.getElementById('tunnelHelpText') || 
                    ServicesManager.createTunnelHelpText();
                
                if (this.value) {
                    helpText.innerHTML = `
                        <small style="color: var(--color-text-secondary);">
                            Service IP will be automatically added as a /32 route to tunnel "${this.value}".
                            <br><strong>Note:</strong> The tunnel will be restarted to activate the route immediately.
                        </small>
                    `;
                    helpText.classList.remove('hidden');
                } else {
                    helpText.classList.add('hidden');
                }
            });
        }

        form.addEventListener('submit', async (e) => {
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

            // Validate upstream URL
            try {
                new URL(service.upstream);
            } catch (e) {
                window.Utils.showAlert('Please enter a valid upstream URL (e.g., http://192.168.1.100:8080)', 'error');
                return;
            }
            
            try {
                // Show loading state
                const submitButton = form.querySelector('button[type="submit"]');
                const originalText = submitButton.textContent;
                
                if (service.tunnel) {
                    submitButton.textContent = 'Adding Service & Restarting Tunnel...';
                    window.Utils.showAlert('Adding service with tunnel route (tunnel will restart)...', 'info');
                } else {
                    submitButton.textContent = 'Adding Service...';
                }
                
                submitButton.disabled = true;

                await window.APIClient.addService(service);
                
                let successMessage;
                if (service.tunnel) {
                    successMessage = `Service "${service.name}" added successfully with route to tunnel "${service.tunnel}"! Tunnel has been restarted to activate the route. 🎉`;
                } else {
                    successMessage = `Service "${service.name}" added successfully! 🎉`;
                }
                
                window.Utils.showAlert(successMessage, 'success');
                form.reset();
                
                // Hide tunnel help text
                const helpText = document.getElementById('tunnelHelpText');
                if (helpText) {
                    helpText.classList.add('hidden');
                }
                
                ServicesManager.loadServices();
                
                // Reload tunnels to show updated routes and status
                if (service.tunnel && window.TunnelsManager) {
                    // Small delay to allow tunnel restart to complete
                    setTimeout(() => {
                        window.TunnelsManager.loadTunnels();
                    }, 2000);
                }

                // Restore button
                submitButton.textContent = originalText;
                submitButton.disabled = false;
                
            } catch (error) {
                console.error('Failed to add service:', error);
                
                let errorMessage = `Failed to add service: ${error.message}`;
                if (error.message.includes('tunnel')) {
                    errorMessage += '\n\nNote: The service may have been created but the tunnel restart failed. Check the tunnel status.';
                }
                
                window.Utils.showAlert(errorMessage, 'error');
                
                // Restore button
                const submitButton = form.querySelector('button[type="submit"]');
                submitButton.textContent = 'Add Service';
                submitButton.disabled = false;
            }
        });
    }

    static createTunnelHelpText() {
        const tunnelGroup = document.getElementById('serviceTunnel').closest('.form-group');
        const helpText = document.createElement('div');
        helpText.id = 'tunnelHelpText';
        helpText.className = 'hidden';
        helpText.style.marginTop = '0.5rem';
        tunnelGroup.appendChild(helpText);
        return helpText;
    }
}

// Export to global scope
window.ServicesManager = ServicesManager;