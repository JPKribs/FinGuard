
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
        return confirm(`Delete tunnel "${name}"? This will also remove any services using this tunnel.`);
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
            <h5>Peer ${index + 1} <button type="button" class="btn-danger btn-small" onclick="window.TunnelsManager.removePeer(${index})">Remove</button></h5>
            <div class="form-group">
                <label>Peer Name</label>
                <input type="text" class="peer-name" placeholder="Server ${index + 1}">
            </div>
            <div class="form-group">
                <label>Public Key</label>
                <textarea class="peer-public-key" placeholder="WGqWDWQ3aQLK6tD7dlKud4PCK515UpopGgAGc2f7FS4=" rows="2" required></textarea>
            </div>
            <div class="form-group">
                <label>Endpoint</label>
                <input type="text" class="peer-endpoint" placeholder="server.example.com:51820" required>
            </div>
            <div class="form-group">
                <label>Allowed IPs (comma-separated)</label>
                <input type="text" class="peer-allowed-ips" placeholder="0.0.0.0/0" required>
            </div>
            <div class="form-group">
                <label>Persistent Keepalive (seconds)</label>
                <input type="number" class="peer-keepalive" placeholder="25" value="25" min="0" max="65535">
            </div>
            <div class="form-group">
                <label>Preshared Key (optional)</label>
                <textarea class="peer-preshared" placeholder="Optional preshared key" rows="2"></textarea>
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

    // FORM MANAGEMENT

    // MARK: initializeForm
    static initializeForm() {
        const form = document.getElementById('tunnelForm');
        if (!form) {
            console.error('Tunnel form not found');
            return;
        }

        form.addEventListener('submit', this.handleFormSubmit.bind(this));
    }

    // MARK: handleFormSubmit
    static async handleFormSubmit(e) {
        e.preventDefault();
        
        this.scrollToTop();
        
        const peers = this.collectPeerData();
        if (!peers) return;
        
        const tunnel = this.collectTunnelData(peers);
        
        if (!this.validateTunnel(tunnel, peers)) return;
        
        try {
            await this.createTunnel(tunnel);
        } catch (error) {
            this.handleTunnelCreationError(error);
        }
    }

    // MARK: collectPeerData
    static collectPeerData() {
        const peers = [];
        const peerConfigs = document.querySelectorAll('.peer-config');
        
        for (let peerConfig of peerConfigs) {
            const peer = this.extractPeerData(peerConfig);
            
            if (!this.validatePeerData(peer)) {
                return null;
            }
            
            peers.push(peer);
        }
        
        return peers;
    }

    // MARK: extractPeerData
    static extractPeerData(peerConfig) {
        const publicKey = peerConfig.querySelector('.peer-public-key').value.trim();
        const endpoint = peerConfig.querySelector('.peer-endpoint').value.trim();
        const allowedIPs = peerConfig.querySelector('.peer-allowed-ips').value.trim();
        const keepaliveValue = parseInt(peerConfig.querySelector('.peer-keepalive').value) || 0;
        
        return {
            name: peerConfig.querySelector('.peer-name').value.trim() || 'Peer',
            public_key: publicKey,
            endpoint: endpoint,
            allowed_ips: allowedIPs.split(',').map(ip => ip.trim()).filter(ip => ip),
            persistent_keepalive: keepaliveValue > 0 ? keepaliveValue : 0,
            preshared_key: peerConfig.querySelector('.peer-preshared').value.trim()
        };
    }

    // MARK: validatePeerData
    static validatePeerData(peer) {
        if (!peer.public_key || !peer.endpoint || !peer.allowed_ips.length) {
            window.Utils.showAlert('All peer fields are required', 'error');
            return false;
        }
        return true;
    }

    // MARK: collectTunnelData
    static collectTunnelData(peers) {
        const monitorInterval = parseInt(document.getElementById('tunnelMonitorInterval')?.value) || 30;
        const staleTimeout = parseInt(document.getElementById('tunnelStaleTimeout')?.value) || 300;
        const reconnectRetries = parseInt(document.getElementById('tunnelReconnectRetries')?.value) || 3;
        
        return {
            name: document.getElementById('tunnelName').value.trim(),
            listen_port: parseInt(document.getElementById('tunnelListenPort')?.value) || 0,
            private_key: document.getElementById('tunnelPrivateKey').value.trim(),
            mtu: parseInt(document.getElementById('tunnelMTU').value) || 1420,
            addresses: [document.getElementById('tunnelAddress').value.trim()].filter(addr => addr),
            routes: [],
            peers: peers,
            monitor_interval: monitorInterval,
            stale_connection_timeout: staleTimeout,
            reconnection_retries: reconnectRetries
        };
    }

    // MARK: createTunnel
    static async createTunnel(tunnel) {
        window.Utils.showAlert('Creating tunnel...', 'info');
        
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
        this.resetFirstPeerConfig(peerSection);
        
        window.FinGuardConfig.peerIndex = 1;
    }

    // MARK: removeExtraPeerConfigs
    static removeExtraPeerConfigs(peerSection) {
        const peerConfigs = peerSection.querySelectorAll('.peer-config');
        for (let i = 1; i < peerConfigs.length; i++) {
            peerConfigs[i].remove();
        }
    }

    // MARK: resetFirstPeerConfig
    static resetFirstPeerConfig(peerSection) {
        const firstPeer = peerSection.querySelector('.peer-config[data-peer-index="0"]');
        if (firstPeer) {
            firstPeer.querySelector('.peer-name').value = '';
            firstPeer.querySelector('.peer-public-key').value = '';
            firstPeer.querySelector('.peer-endpoint').value = '';
            firstPeer.querySelector('.peer-allowed-ips').value = '';
            firstPeer.querySelector('.peer-keepalive').value = '25';
            firstPeer.querySelector('.peer-preshared').value = '';
        }
    }

    // MARK: focusFirstField
    static focusFirstField() {
        const firstField = document.getElementById('tunnelName');
        if (firstField) {
            firstField.focus();
        }
    }

    // MARK: scrollToTop
    static scrollToTop() {
        window.scrollTo({ top: 0, behavior: 'smooth' });
        
        const container = document.querySelector('.container');
        if (container) {
            container.scrollTop = 0;
        }
    }

    // VALIDATION

    // MARK: validateTunnel
    static validateTunnel(tunnel, peers) {
        return this.validateBasicFields(tunnel) &&
               this.validatePeers(peers) &&
               this.validateTunnelName(tunnel.name) &&
               this.validatePrivateKey(tunnel.private_key) &&
               this.validateAddresses(tunnel.addresses) &&
               this.validatePeerAllowedIPs(peers);
    }

    // MARK: validateBasicFields
    static validateBasicFields(tunnel) {
        if (!tunnel.name) {
            window.Utils.showAlert('Tunnel name is required', 'error');
            return false;
        }
        
        if (!tunnel.private_key) {
            window.Utils.showAlert('Private key is required', 'error');
            return false;
        }
        
        if (tunnel.addresses.length === 0) {
            window.Utils.showAlert('At least one address is required', 'error');
            return false;
        }
        
        return true;
    }

    // MARK: validatePeers
    static validatePeers(peers) {
        if (peers.length === 0) {
            window.Utils.showAlert('At least one peer is required', 'error');
            return false;
        }
        return true;
    }

    // MARK: validateTunnelName
    static validateTunnelName(name) {
        if (!/^[a-zA-Z0-9-_]+$/.test(name)) {
            window.Utils.showAlert('Tunnel name can only contain letters, numbers, hyphens, and underscores', 'error');
            return false;
        }
        return true;
    }

    // MARK: validatePrivateKey
    static validatePrivateKey(privateKey) {
        if (privateKey.length !== 44 || !/^[A-Za-z0-9+/]+=*$/.test(privateKey)) {
            window.Utils.showAlert('Invalid private key format. Should be 44 characters base64-encoded.', 'error');
            return false;
        }
        return true;
    }

    // MARK: validateAddresses
    static validateAddresses(addresses) {
        for (const addr of addresses) {
            if (!this.isValidCIDR(addr)) {
                window.Utils.showAlert(`Invalid address format: ${addr}. Use CIDR notation (e.g., 10.0.0.1/24)`, 'error');
                return false;
            }
        }
        return true;
    }

    // MARK: validatePeerAllowedIPs
    static validatePeerAllowedIPs(peers) {
        for (const peer of peers) {
            for (const allowedIP of peer.allowed_ips) {
                if (!this.isValidCIDR(allowedIP)) {
                    window.Utils.showAlert(`Invalid allowed IP format for peer ${peer.name}: ${allowedIP}`, 'error');
                    return false;
                }
            }
        }
        return true;
    }

    // MARK: isValidCIDR
    static isValidCIDR(cidr) {
        const cidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;
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
}

// GLOBAL SCOPE EXPORT

window.TunnelsManager = TunnelsManager;