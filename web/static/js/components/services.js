
// SERVICES MANAGEMENT

class ServicesManager {
    // SERVICES LOADING

    // MARK: loadServices
    static async loadServices() {
        try {
            window.Utils.showLoading('servicesList');
            
            const responses = await this.fetchServicesData();
            this.processServicesData(responses);
        } catch (error) {
            this.handleServicesLoadError(error);
        }
    }

    // MARK: fetchServicesData
    static async fetchServicesData() {
        return await Promise.all([
            window.APIClient.getServices(),
            window.APIClient.getTunnels()
        ]);
    }

    // MARK: processServicesData
    static processServicesData([servicesResponse, tunnelsResponse]) {
        const services = servicesResponse.data || [];
        const tunnels = tunnelsResponse.data || [];
        
        this.renderServicesList(services);
        this.updateTunnelsDropdown(tunnels);
    }

    // MARK: handleServicesLoadError
    static handleServicesLoadError(error) {
        console.error('Failed to load services:', error);
        
        if (!error.message.includes('Authentication')) {
            const servicesList = document.getElementById('servicesList');
            servicesList.innerHTML = '<p style="color: var(--color-danger);">Failed to load services</p>';
        }
    }

    // SERVICES RENDERING

    // MARK: renderServicesList
    static renderServicesList(services) {
        const servicesList = document.getElementById('servicesList');

        if (services.length === 0) {
            this.renderEmptyServicesList(servicesList);
            return;
        }

        servicesList.innerHTML = services.map(service => this.generateServiceHTML(service)).join('');
    }

    // MARK: renderEmptyServicesList
    static renderEmptyServicesList(container) {
        container.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No services configured</p>';
    }

    // MARK: generateServiceHTML
    static generateServiceHTML(service) {
        const infoRows = this.buildServiceInfoRows(service);

        return `
            <div class="list-item" style="display: flex; position: relative;">
                <div style="flex: 1; display: flex; flex-direction: column;">
                    <strong>${window.Utils.escapeHtml(service.name)}.local</strong>
                    ${this.generateInfoRowsHTML(infoRows)}
                </div>
                <div style="display: flex; flex-direction: column; justify-content: space-between; align-items: flex-end; margin-left: 1rem; padding-left: 1rem; border-left: 1px solid var(--color-border); align-self: stretch;">
                    <span class="status ${service.status === 'running' ? 'running' : 'stopped'}">${service.status}</span>
                    <button class="btn-danger btn-small" onclick="window.ServicesManager.deleteService('${window.Utils.escapeHtml(service.name)}')">Delete</button>                   
                </div>
            </div>
        `;
    }

    // MARK: buildServiceInfoRows
    static buildServiceInfoRows(service) {
        const infoRows = [
            { label: 'Upstream', value: service.upstream }
        ];

        if (service.tunnel) {
            infoRows.push({ label: 'Tunnel', value: service.tunnel });
        }

        infoRows.push(
            { label: 'Jellyfin', value: service.jellyfin ? '✓' : '✗' },
            { label: 'WebSocket', value: service.websocket ? '✓' : '✗' },
            { label: 'Default', value: service.default ? '✓' : '✗' },
            { label: 'mDNS', value: service.publish_mdns ? '✓' : '✗' }
        );

        return infoRows;
    }

    // MARK: generateInfoRowsHTML
    static generateInfoRowsHTML(infoRows) {
        return infoRows.map(row => `
            <div style="display: flex; justify-content: space-between;">
                <small style="color: var(--color-text-secondary);">${row.label}:</small>
                <small style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block;">
                    ${window.Utils.escapeHtml(String(row.value))}
                </small>
            </div>
        `).join('');
    }

    // MARK: extractIPFromURL
    static extractIPFromURL(url) {
        try {
            const urlObj = new URL(url);
            const hostname = urlObj.hostname;
            
            const ipRegex = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
            return ipRegex.test(hostname) ? hostname : hostname;
        } catch (e) {
            return null;
        }
    }

    // DROPDOWN MANAGEMENT

    // MARK: updateTunnelsDropdown
    static updateTunnelsDropdown(tunnels) {
        const tunnelSelect = document.getElementById('serviceTunnel');
        tunnelSelect.innerHTML = this.generateTunnelOptions(tunnels);
    }

    // MARK: generateTunnelOptions
    static generateTunnelOptions(tunnels) {
        const noneOption = '<option value="">None</option>';
        const tunnelOptions = tunnels.map(tunnel => 
            `<option value="${window.Utils.escapeHtml(tunnel.name)}" title="Service IP will be added as route to this tunnel">
                ${window.Utils.escapeHtml(tunnel.name)}
            </option>`
        ).join('');
        
        return noneOption + tunnelOptions;
    }

    // SERVICE OPERATIONS

    // MARK: deleteService
    static async deleteService(name) {
        if (!this.confirmServiceDeletion(name)) return;
        
        try {
            await window.APIClient.deleteService(name);
            this.handleDeleteSuccess(name);
        } catch (error) {
            this.handleDeleteError(error, name);
        }
    }

    // MARK: confirmServiceDeletion
    static confirmServiceDeletion(name) {
        return confirm(`Delete service "${name}"?\n\nThis will also remove any associated routes from tunnels.`);
    }

    // MARK: handleDeleteSuccess
    static handleDeleteSuccess(name) {
        window.Utils.showAlert(`Service "${name}" successfully deleted`, 'success');
        this.loadServices();
        
        if (window.TunnelsManager) {
            window.TunnelsManager.loadTunnels();
        }
    }

    // MARK: handleDeleteError
    static handleDeleteError(error, name) {
        console.error('Failed to delete service:', error);
        window.Utils.showAlert(`Failed to delete service "${name}": ${error.message}`, 'error');
    }

    // FORM MANAGEMENT

    // MARK: initializeServiceForm
    static initializeServiceForm() {
        this.setupJellyfinLogic();
        this.setupFormSubmission();
        this.setupTunnelHelp();
    }

    // MARK: setupFormSubmission
    static setupFormSubmission() {
        const form = document.getElementById('serviceForm');
        if (!form) {
            console.error('Service form not found');
            return;
        }

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const service = this.collectServiceData();
            
            if (!this.validateServiceData(service)) {
                return;
            }
            
            try {
                await this.createService(service);
            } catch (error) {
                this.handleServiceCreationError(error);
            }
        });
    }

    // MARK: setupJellyfinLogic
    static setupJellyfinLogic() {
        const jellyfinCheckbox = document.getElementById('serviceJellyfin');
        const websocketCheckbox = document.getElementById('serviceWebsocket');
        
        if (jellyfinCheckbox && websocketCheckbox) {
            jellyfinCheckbox.addEventListener('change', function() {
                if (this.checked) {
                    websocketCheckbox.checked = true;
                    websocketCheckbox.disabled = true;
                } else {
                    websocketCheckbox.disabled = false;
                }
            });
        }
    }

    // MARK: setupTunnelHelp
    static setupTunnelHelp() {
        const tunnelSelect = document.getElementById('serviceTunnel');
        if (tunnelSelect) {
            tunnelSelect.addEventListener('change', this.handleTunnelSelectChange.bind(this));
        }
    }

    // MARK: handleTunnelSelectChange
    static handleTunnelSelectChange(event) {
        const helpText = document.getElementById('tunnelHelpText') || this.createTunnelHelpText();
        
        if (event.target.value) {
            this.showTunnelHelp(helpText, event.target.value);
        } else {
            this.hideTunnelHelp(helpText);
        }
    }

    // MARK: showTunnelHelp
    static showTunnelHelp(helpText, tunnelName) {
        helpText.innerHTML = `
            <small style="color: var(--color-text-secondary);">
                Service IP will be automatically added as a /32 route to tunnel "${tunnelName}".
                <br><strong>Note:</strong> The tunnel will be restarted to activate the route immediately.
            </small>
        `;
        helpText.classList.remove('hidden');
    }

    // MARK: hideTunnelHelp
    static hideTunnelHelp(helpText) {
        helpText.classList.add('hidden');
    }

    // MARK: createTunnelHelpText
    static createTunnelHelpText() {
        const tunnelGroup = document.getElementById('serviceTunnel').closest('.form-group');
        const helpText = document.createElement('div');
        helpText.id = 'tunnelHelpText';
        helpText.className = 'hidden';
        helpText.style.marginTop = '0.5rem';
        tunnelGroup.appendChild(helpText);
        return helpText;
    }

    // MARK: collectServiceData
    static collectServiceData() {
        const jellyfinChecked = document.getElementById('serviceJellyfin').checked;
        
        return {
            name: document.getElementById('serviceName').value.trim(),
            upstream: document.getElementById('serviceUpstream').value.trim(),
            jellyfin: jellyfinChecked,
            websocket: document.getElementById('serviceWebsocket').checked,
            default: document.getElementById('serviceDefault').checked,
            publish_mdns: document.getElementById('serviceMDNS').checked,
            tunnel: document.getElementById('serviceTunnel').value || undefined
        };
    }

    // MARK: validateServiceData
    static validateServiceData(service) {
        if (!service.name || !service.upstream) {
            window.Utils.showAlert('Service name and upstream URL are required', 'error');
            return false;
        }
        
        if (!this.validateServiceName(service.name)) {
            return false;
        }

        if (!this.validateUpstreamURL(service.upstream)) {
            return false;
        }

        return true;
    }

    // MARK: validateServiceName
    static validateServiceName(name) {
        if (!/^[a-zA-Z0-9-]+$/.test(name)) {
            window.Utils.showAlert('Service name can only contain letters, numbers, and hyphens', 'error');
            return false;
        }
        return true;
    }

    // MARK: validateUpstreamURL
    static validateUpstreamURL(upstream) {
        try {
            new URL(upstream);
            return true;
        } catch (e) {
            window.Utils.showAlert('Please enter a valid upstream URL (e.g., http://192.168.1.100:8080)', 'error');
            return false;
        }
    }

    // MARK: createService
    static async createService(service) {
        const form = document.getElementById('serviceForm');
        const submitButton = this.updateSubmitButton(form, service.tunnel);

        try {
            await window.APIClient.addService(service);
            this.handleServiceCreationSuccess(service, form);
        } finally {
            this.restoreSubmitButton(submitButton);
        }
    }

    // MARK: updateSubmitButton
    static updateSubmitButton(form, hasTunnel) {
        const submitButton = form.querySelector('button[type="submit"]');
        const originalText = submitButton.textContent;
        
        if (hasTunnel) {
            submitButton.textContent = 'Adding Service & Restarting Tunnel...';
            window.Utils.showAlert('Adding service with tunnel route (tunnel will restart)...', 'info');
        } else {
            submitButton.textContent = 'Adding Service...';
        }
        
        submitButton.disabled = true;
        
        return { button: submitButton, originalText };
    }

    // MARK: handleServiceCreationSuccess
    static handleServiceCreationSuccess(service, form) {
        const successMessage = this.buildSuccessMessage(service);
        
        window.Utils.showAlert(successMessage, 'success');
        
        this.resetFormAfterSuccess(form);
        this.reloadServiceData(service);
    }

    // MARK: buildSuccessMessage
    static buildSuccessMessage(service) {
        if (service.tunnel) {
            return `Service "${service.name}" added successfully with route to tunnel "${service.tunnel}"! Tunnel has been restarted to activate the route.`;
        }
        return `Service "${service.name}" added successfully!`;
    }

    // MARK: resetFormAfterSuccess
    static resetFormAfterSuccess(form) {
        form.reset();
        
        const websocketCheckbox = document.getElementById('serviceWebsocket');
        if (websocketCheckbox) {
            websocketCheckbox.disabled = false;
        }
        
        const helpText = document.getElementById('tunnelHelpText');
        if (helpText) {
            helpText.classList.add('hidden');
        }
    }

    // MARK: reloadServiceData
    static reloadServiceData(service) {
        this.loadServices();
        
        if (service.tunnel && window.TunnelsManager) {
            setTimeout(() => {
                window.TunnelsManager.loadTunnels();
            }, 2000);
        }
    }

    // MARK: restoreSubmitButton
    static restoreSubmitButton({ button, originalText }) {
        button.textContent = originalText;
        button.disabled = false;
    }

    // MARK: handleServiceCreationError
    static handleServiceCreationError(error) {
        console.error('Failed to add service:', error);
        
        let errorMessage = `Failed to add service: ${error.message}`;
        if (error.message.includes('tunnel')) {
            errorMessage += '\n\nNote: The service may have been created but the tunnel restart failed. Check the tunnel status.';
        }
        
        window.Utils.showAlert(errorMessage, 'error');
    }

    // MARK: initialize
    static initialize() {
        this.initializeServiceForm();
        this.loadServices();
    }
}

// AUTO-INITIALIZE WHEN DOM IS READY
document.addEventListener('DOMContentLoaded', function() {
    if (window.ServicesManager) {
        window.ServicesManager.initialize();
    }
});

// GLOBAL SCOPE EXPORT
window.ServicesManager = ServicesManager;