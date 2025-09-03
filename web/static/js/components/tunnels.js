// WIREGUARD TUNNELS MANAGEMENT

class TunnelsManager {
    // TUNNEL LOADING

    // MARK: loadTunnels
    static async loadTunnels() {
        try {
            window.Utils.showLoading('tunnelsList');
            
            const response = await window.APIClient.getTunnels();
            const tunnels = response.data || [];
            
            this.renderTunnelsList(tunnels);
        } catch (error) {
            this.handleTunnelsLoadError(error);
        }
    }

    // MARK: handleTunnelsLoadError
    static handleTunnelsLoadError(error) {
        console.error('Failed to load tunnels:', error);
        
        if (!error.message.includes('Authentication')) {
            const tunnelsList = document.getElementById('tunnelsList');
            tunnelsList.innerHTML = '<p style="color: var(--color-danger);">Failed to load tunnels</p>';
        }
    }

    // TUNNEL RENDERING

    // MARK: renderTunnelsList
    static renderTunnelsList(tunnels) {
        const tunnelsList = document.getElementById('tunnelsList');

        if (tunnels.length === 0) {
            this.renderEmptyTunnelsList(tunnelsList);
            return;
        }

        tunnelsList.innerHTML = tunnels.map(tunnel => this.generateTunnelHTML(tunnel)).join('');
    }

    // MARK: renderEmptyTunnelsList
    static renderEmptyTunnelsList(container) {
        container.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No tunnels configured</p>';
    }

    // MARK: generateTunnelHTML
    static generateTunnelHTML(tunnel) {
        const infoRows = this.buildTunnelInfoRows(tunnel);
        const isRunning = tunnel.state === 'running';

        return `
            <div class="list-item" style="display: flex; position: relative;">
                <div style="flex: 1; display: flex; flex-direction: column;">
                    <strong>${window.Utils.escapeHtml(tunnel.name)}</strong>
                    ${this.generateInfoRowsHTML(infoRows)}
                </div>
                <div style="display: flex; flex-direction: column; justify-content: space-between; align-items: flex-end; margin-left: 1rem; padding-left: 1rem; border-left: 1px solid var(--color-border); align-self: stretch; gap: 0.5rem;">
                    <span class="status ${isRunning ? 'running' : 'stopped'}">${tunnel.state}</span>
                    ${this.generateTunnelActionsHTML(tunnel, isRunning)}
                </div>
            </div>
        `;
    }

    // MARK: buildTunnelInfoRows
    static buildTunnelInfoRows(tunnel) {
        const infoRows = [];
        
        if (tunnel.interface) infoRows.push({ label: 'Interface', value: tunnel.interface });
        infoRows.push({ label: 'Peers', value: tunnel.peers || 0 });
        infoRows.push({ label: 'MTU', value: tunnel.mtu || 'N/A' });
        
        if (tunnel.routes && tunnel.routes.length > 0) {
            const routeText = this.formatRoutes(tunnel.routes);
            infoRows.push({ label: 'Routes', value: routeText });
        }

        return infoRows;
    }

    // MARK: formatRoutes
    static formatRoutes(routes) {
        if (routes.length > 2) {
            return `${routes.slice(0, 2).join(', ')} +${routes.length - 2} more`;
        }
        return routes.join(', ');
    }

    // MARK: generateInfoRowsHTML
    static generateInfoRowsHTML(infoRows) {
        return infoRows.map(row => `
            <div style="display: flex; justify-content: space-between;">
                <small style="color: var(--color-text-secondary);">${row.label}:</small>
                <small style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block;" title="${window.Utils.escapeHtml(String(row.value))}">
                    ${window.Utils.escapeHtml(String(row.value))}
                </small>
            </div>
        `).join('');
    }

    // MARK: generateTunnelActionsHTML
    static generateTunnelActionsHTML(tunnel, isRunning) {
        const escapedName = window.Utils.escapeHtml(tunnel.name);
        const restartButton = isRunning ?
            `<button class="btn-small" onclick="window.TunnelsManager.restartTunnel('${escapedName}')" title="Restart tunnel to apply route changes">Restart</button>` : 
            '';

        return `
            <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                ${restartButton}
                <button class="btn-danger btn-small" onclick="window.TunnelsManager.deleteTunnel('${escapedName}')">Delete</button>
            </div>
        `;
    }

    // TUNNEL OPERATIONS

    // MARK: restartTunnel
    static async restartTunnel(name) {
        if (!this.confirmTunnelRestart(name)) return;
        
        try {
            await this.performTunnelRestart(name);
            this.handleRestartSuccess(name);
        } catch (error) {
            this.handleTunnelError(error, 'restart', name);
        }
    }

    // MARK: confirmTunnelRestart
    static confirmTunnelRestart(name) {
        return confirm(`Restart tunnel "${name}"?\n\nThis will briefly disconnect the tunnel to apply any route changes.`);
    }

    // MARK: performTunnelRestart
    static async performTunnelRestart(name) {
        window.Utils.showAlert(`Restarting tunnel "${name}"...`, 'info');
        await window.APIClient.restartTunnel(name);
    }

    // MARK: handleRestartSuccess
    static handleRestartSuccess(name) {
        window.Utils.showAlert(`Tunnel "${name}" restarted successfully!`, 'success');
        setTimeout(() => this.loadTunnels(), 1000);
    }

    // MARK: deleteTunnel
    static async deleteTunnel(name) {
        if (!this.confirmTunnelDeletion(name)) return;
        
        try {
            await window.APIClient.deleteTunnel(name);
            this.handleDeleteSuccess(name);
        } catch (error) {
            this.handleTunnelError(error, 'delete', name);
        }
    }

    // MARK: confirmTunnelDeletion
    static confirmTunnelDeletion(name) {
        return confirm(`Delete tunnel "${name}"?\n\nThis will also remove any services using this tunnel.`);
    }

    // MARK: handleDeleteSuccess
    static handleDeleteSuccess(name) {
        window.Utils.showAlert(`Tunnel "${name}" deleted successfully`, 'success');
        this.loadTunnels();
        if (window.ServicesManager) {
            window.ServicesManager.loadServices();
        }
    }

    // MARK: handleTunnelError
    static handleTunnelError(error, operation, name) {
        console.error(`Failed to ${operation} tunnel:`, error);
        window.Utils.showAlert(`Failed to ${operation} tunnel "${name}": ${error.message}`, 'error');
    }

    // PEER MANAGEMENT

    // MARK: addPeer
    static addPeer() {
        const peerSection = document.getElementById('peerSection');
        
        if (!peerSection) return;
        
        const currentIndex = window.FinGuardConfig.peerIndex;
        const peerConfig = this.createPeerElement(currentIndex);
        peerSection.appendChild(peerConfig);
        
        window.FinGuardConfig.peerIndex++;
    }

    // MARK: createPeerElement
    static createPeerElement(index) {
        const peerConfig = document.createElement('div');
        peerConfig.className = 'peer-config';
        peerConfig.setAttribute('data-peer-index', index);
        peerConfig.innerHTML = this.generatePeerHTML(index);
        return peerConfig;
    }

    // MARK: generatePeerHTML
    static generatePeerHTML(index) {
        return `
            <h5 style="padding-bottom: 10px;">
                Peer ${index + 1}
                <button type="button" class="btn-danger btn-small" style="width: auto; min-width: 60px;" onclick="window.TunnelsManager.removePeer(${index})">
                    Remove
                </button>
            </h5>
            <div class="form-group">
                <label>Peer Name</label>
                <input type="text" class="peer-name" placeholder="Enter peer name">
            </div>
            <div class="form-group">
                <label>Public Key</label>
                <input type="text" class="peer-publickey" placeholder="Enter peer public key" required>
            </div>
            <div class="form-group">
                <label>Endpoint</label>
                <input type="text" class="peer-endpoint" placeholder="host:port">
            </div>
            <div class="form-group">
                <label>Allowed IPs</label>
                <input type="text" class="peer-allowedips" placeholder="0.0.0.0/0 or specific IPs/subnets">
            </div>
            <div class="form-group">
                <label>Pre-shared Key</label>
                <input type="text" class="peer-presharedkey" placeholder="Optional pre-shared key">
            </div>
            <div class="form-group">
                <label>Persistent Keepalive</label>
                <input type="number" class="peer-keepalive" placeholder="0" min="0" max="65535">
            </div>
        `;
    }

    // MARK: removePeer
    static removePeer(index) {
        const peerConfig = document.querySelector(`[data-peer-index="${index}"]`);
        if (peerConfig) {
            peerConfig.remove();
        }
    }

    // TUNNEL FORM HANDLING

    // MARK: initializeTunnelForm
    static initializeTunnelForm() {
        const form = document.getElementById('tunnelForm');
        if (!form) return;

        form.addEventListener('submit', this.handleTunnelFormSubmit.bind(this));

        // Clear any existing peers first
        const peerSection = document.getElementById('peerSection');
        if (peerSection) {
            this.removeExtraPeerConfigs(peerSection);
        }

        // Initialize peer index and add first peer
        if (!window.FinGuardConfig) {
            window.FinGuardConfig = {};
        }
        window.FinGuardConfig.peerIndex = 0;

        this.addPeer();
        this.setupFormValidation();
    }

    // MARK: setupFormValidation
    static setupFormValidation() {
        const form = document.getElementById('tunnelForm');
        if (!form) return;

        const validateField = (field, validationFn, errorMsg) => {
            field.addEventListener('blur', () => {
                const isValid = validationFn(field.value);
                this.toggleFieldError(field, !isValid, errorMsg);
            });
        };

        const addressesField = document.getElementById('tunnelAddresses');
        if (addressesField) {
            validateField(addressesField, (value) => {
                if (!value.trim()) return false;
                return value.split(',').every(addr => this.isValidCIDR(addr.trim()));
            }, 'Enter valid CIDR addresses (e.g., 10.0.0.1/24)');
        }

        const routesField = document.getElementById('tunnelRoutes');
        if (routesField) {
            validateField(routesField, (value) => {
                if (!value.trim()) return true;
                return value.split(',').every(route => this.isValidCIDR(route.trim()));
            }, 'Enter valid CIDR routes (e.g., 10.0.0.0/24)');
        }
    }

    // MARK: toggleFieldError
    static toggleFieldError(field, hasError, message) {
        const existingError = field.parentNode.querySelector('.field-error');
        
        if (hasError) {
            if (!existingError) {
                const errorDiv = document.createElement('div');
                errorDiv.className = 'field-error';
                errorDiv.style.color = 'var(--color-danger)';
                errorDiv.style.fontSize = '0.8em';
                errorDiv.style.marginTop = '0.25rem';
                errorDiv.textContent = message;
                field.parentNode.appendChild(errorDiv);
            }
            field.style.borderColor = 'var(--color-danger)';
        } else {
            if (existingError) {
                existingError.remove();
            }
            field.style.borderColor = '';
        }
    }

    // MARK: handleTunnelFormSubmit
    static async handleTunnelFormSubmit(e) {
        e.preventDefault();
        
        const submitButton = e.target.querySelector('button[type="submit"]');
        const originalText = submitButton.textContent;
        
        try {
            this.updateSubmitButton(submitButton, 'Creating Tunnel...');
            
            const tunnelData = this.extractTunnelFormData(e.target);
            
            if (!this.validateTunnelData(tunnelData)) {
                throw new Error('Please check the form for errors');
            }

            await this.createTunnel(tunnelData);
        } catch (error) {
            this.handleTunnelCreationError(error);
        } finally {
            this.restoreSubmitButton(submitButton, originalText);
        }
    }

    // MARK: updateSubmitButton
    static updateSubmitButton(button, text) {
        button.textContent = text;
        button.disabled = true;
    }

    // MARK: restoreSubmitButton
    static restoreSubmitButton(button, originalText) {
        button.textContent = originalText;
        button.disabled = false;
    }

    // MARK: extractTunnelFormData
    static extractTunnelFormData(form) {
        const formData = new FormData(form);
        const peers = this.extractPeerData(form);

        return {
            name: formData.get('tunnelName')?.trim(),
            listen_port: parseInt(formData.get('tunnelListenPort')) || 0,
            private_key: formData.get('tunnelPrivateKey')?.trim(),
            mtu: parseInt(formData.get('tunnelMTU')) || 1420,
            addresses: formData.get('tunnelAddresses')?.split(',').map(addr => addr.trim()).filter(Boolean) || [],
            routes: formData.get('tunnelRoutes')?.split(',').map(route => route.trim()).filter(Boolean) || [],
            peers: peers,
            monitor_interval: parseInt(formData.get('tunnelMonitorInterval')) || 30,
            stale_connection_timeout: parseInt(formData.get('tunnelStaleTimeout')) || 300,
            reconnection_retries: parseInt(formData.get('tunnelReconnectRetries')) || 3
        };
    }

    // MARK: extractPeerData
    static extractPeerData(form) {
        const peerConfigs = form.querySelectorAll('.peer-config');
        const peers = [];

        peerConfigs.forEach(peerConfig => {
            const name = peerConfig.querySelector('.peer-name')?.value?.trim();
            const publicKey = peerConfig.querySelector('.peer-publickey')?.value?.trim();
            const endpoint = peerConfig.querySelector('.peer-endpoint')?.value?.trim();
            const allowedIPs = peerConfig.querySelector('.peer-allowedips')?.value?.trim();
            const presharedKey = peerConfig.querySelector('.peer-presharedkey')?.value?.trim();
            const keepalive = parseInt(peerConfig.querySelector('.peer-keepalive')?.value) || 0;

            if (publicKey) {
                peers.push({
                    name: name || '',
                    public_key: publicKey,
                    endpoint: endpoint || '',
                    allowed_ips: allowedIPs ? allowedIPs.split(',').map(ip => ip.trim()) : [],
                    preshared_key: presharedKey || '',
                    persistent_keepalive: keepalive
                });
            }
        });

        return peers;
    }

    // MARK: validateTunnelData
    static validateTunnelData(data) {
        if (!data.name) {
            window.Utils.showAlert('Tunnel name is required', 'error');
            return false;
        }

        if (!data.private_key) {
            window.Utils.showAlert('Private key is required', 'error');
            return false;
        }

        if (data.peers.length === 0) {
            window.Utils.showAlert('At least one peer is required', 'error');
            return false;
        }

        if (data.addresses.length === 0) {
            window.Utils.showAlert('At least one address is required', 'error');
            return false;
        }

        return true;
    }

    // MARK: createTunnel
    static async createTunnel(tunnel) {
        await window.APIClient.addTunnel(tunnel);
        
        window.Utils.showAlert(`Tunnel "${tunnel.name}" created successfully!`, 'success');
        
        this.clearForm();
        this.loadTunnels();
        this.scrollToTop();
    }

    // MARK: handleTunnelCreationError
    static handleTunnelCreationError(error) {
        console.error('Failed to create tunnel:', error);
        window.Utils.showAlert(`Failed to create tunnel: ${error.message}`, 'error');
        this.scrollToTop();
    }

    // FORM UTILITIES

    // MARK: clearForm
    static clearForm() {
        const form = document.getElementById('tunnelForm');
        if (form) {
            form.reset();
        }
        
        this.resetDefaultValues();
        this.resetPeerConfigs();
        this.focusFirstField();
    }

    // MARK: resetDefaultValues
    static resetDefaultValues() {
        const fieldsWithDefaults = [
            { id: 'tunnelMTU', value: '1420' },
            { id: 'tunnelMonitorInterval', value: '30' },
            { id: 'tunnelStaleTimeout', value: '300' },
            { id: 'tunnelReconnectRetries', value: '3' }
        ];
        
        fieldsWithDefaults.forEach(field => {
            const element = document.getElementById(field.id);
            if (element) {
                element.value = field.value;
            }
        });
    }

    // MARK: resetPeerConfigs
    static resetPeerConfigs() {
        const peerSection = document.getElementById('peerSection');
        if (!peerSection) return;
        
        this.removeExtraPeerConfigs(peerSection);
        
        // Reset to 0 so first peer shows as "Peer 1"
        window.FinGuardConfig.peerIndex = 0;
        
        this.addPeer();
    }

    // MARK: removeExtraPeerConfigs
    static removeExtraPeerConfigs(peerSection) {
        const peerConfigs = peerSection.querySelectorAll('.peer-config');
        peerConfigs.forEach(config => config.remove());
    }

    // MARK: focusFirstField
    static focusFirstField() {
        const firstField = document.getElementById('tunnelName');
        if (firstField) {
            setTimeout(() => firstField.focus(), 100);
        }
    }

    // MARK: scrollToTop
    static scrollToTop() {
        const tunnelSection = document.getElementById('tunnels');
        if (tunnelSection) {
            tunnelSection.scrollIntoView({ behavior: 'smooth' });
        }
    }

    // VALIDATION UTILITIES

    // MARK: isValidCIDR
    static isValidCIDR(cidr) {
        const cidrRegex = /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\/(?:[0-9]|[1-2][0-9]|3[0-2])$/;
        if (!cidrRegex.test(cidr)) {
            return false;
        }
        
        const [ip, prefix] = cidr.split('/');
        const octets = ip.split('.');
        
        for (const octet of octets) {
            const num = parseInt(octet);
            if (num < 0 || num > 255) {
                return false;
            }
        }
        
        const prefixNum = parseInt(prefix);
        return prefixNum >= 0 && prefixNum <= 32;
    }

    // MARK: resetForm
    static resetForm() {
        this.clearForm();
    }

    // MARK: initialize
    static initialize() {
        // Ensure clean initialization
        if (!window.FinGuardConfig) {
            window.FinGuardConfig = {};
        }
        
        this.initializeTunnelForm();
        this.loadTunnels();
    }
}

// GLOBAL SCOPE EXPORT
window.TunnelsManager = TunnelsManager;

window.addPeer = function() {
    if (window.TunnelsManager) {
        window.TunnelsManager.addPeer();
    }
};