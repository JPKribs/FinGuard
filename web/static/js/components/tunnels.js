// WireGuard Tunnels Management
class TunnelsManager {
    static async loadTunnels() {
        try {
            window.Utils.showLoading('tunnelsList');
            
            const response = await window.APIClient.getTunnels();
            const tunnels = response.data || [];
            
            TunnelsManager.renderTunnelsList(tunnels);
            
        } catch (error) {
            console.error('Failed to load tunnels:', error);
            if (!error.message.includes('Authentication')) {
                document.getElementById('tunnelsList').innerHTML = '<p style="color: var(--color-danger);">Failed to load tunnels</p>';
            }
        }
    }

    // MARK: renderTunnelsList
    static renderTunnelsList(tunnels) {
        const tunnelsList = document.getElementById('tunnelsList');

        if (tunnels.length === 0) {
            tunnelsList.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No tunnels configured</p>';
            return;
        }

        tunnelsList.innerHTML = tunnels.map(tunnel => {
            const infoRows = [];
            if (tunnel.interface) infoRows.push({ label: 'Interface', value: tunnel.interface });
            infoRows.push({ label: 'Peers', value: tunnel.peers || 0 });
            infoRows.push({ label: 'MTU', value: tunnel.mtu || 'N/A' });
            if (tunnel.routes && tunnel.routes.length > 0) {
                const routeText = tunnel.routes.length > 2 ? 
                    `${tunnel.routes.slice(0, 2).join(', ')} +${tunnel.routes.length - 2} more` : 
                    tunnel.routes.join(', ');
                infoRows.push({ label: 'Routes', value: routeText });
            }

            const isRunning = tunnel.state === 'running';
            
            return `
                <div class="list-item" style="display: flex; position: relative;">

                    <!-- Left column -->
                    <div style="flex: 1; display: flex; flex-direction: column;">
                        <strong>${window.Utils.escapeHtml(tunnel.name)}</strong>
                        ${infoRows.map(row => `
                            <div style="display: flex; justify-content: space-between;">
                                <small style="color: var(--color-text-secondary);">${row.label}:</small>
                                <small style="color: var(--color-accent); font-weight: bold; font-family: monospace; display: block;" title="${window.Utils.escapeHtml(String(row.value))}">
                                    ${window.Utils.escapeHtml(String(row.value))}
                                </small>
                            </div>
                        `).join('')}
                    </div>

                    <!-- Right column -->
                    <div style="display: flex; flex-direction: column; justify-content: space-between; align-items: flex-end; margin-left: 1rem; padding-left: 1rem; border-left: 1px solid var(--color-border); align-self: stretch; gap: 0.5rem;">
                        <span class="status ${isRunning ? 'running' : 'stopped'}">${tunnel.state}</span>
                        <div style="display: flex; flex-direction: column; gap: 0.25rem;">
                            ${isRunning ? `<button class="btn-small" onclick="window.TunnelsManager.restartTunnel('${window.Utils.escapeHtml(tunnel.name)}')" title="Restart tunnel to apply route changes">Restart</button>` : ''}
                            <button class="btn-danger btn-small" onclick="window.TunnelsManager.deleteTunnel('${window.Utils.escapeHtml(tunnel.name)}')">Delete</button>
                        </div>
                    </div>

                </div>
            `;
        }).join('');
    }

    // MARK: restartTunnel
    static async restartTunnel(name) {
        if (!confirm(`Restart tunnel "${name}"?\n\nThis will briefly disconnect the tunnel to apply any route changes.`)) return;
        
        try {
            window.Utils.showAlert(`Restarting tunnel "${name}"...`, 'info');
            
            await window.APIClient.restartTunnel(name);
            
            window.Utils.showAlert(`Tunnel "${name}" restarted successfully!`, 'success');
            
            // Refresh the tunnel list after a short delay
            setTimeout(() => {
                TunnelsManager.loadTunnels();
            }, 1000);
            
        } catch (error) {
            console.error('Failed to restart tunnel:', error);
            window.Utils.showAlert(`Failed to restart tunnel "${name}": ${error.message}`, 'error');
        }
    }

    static async deleteTunnel(name) {
        if (!confirm(`Delete tunnel "${name}"? This will also remove any services using this tunnel.`)) return;
        
        try {
            await window.APIClient.deleteTunnel(name);
            window.Utils.showAlert(`Tunnel "${name}" deleted successfully`, 'success');
            TunnelsManager.loadTunnels();
            window.ServicesManager.loadServices();
        } catch (error) {
            console.error('Failed to delete tunnel:', error);
            window.Utils.showAlert(`Failed to delete tunnel "${name}": ${error.message}`, 'error');
        }
    }

    // MARK: addPeer
    static addPeer() {
        const peerSection = document.getElementById('peerSection');
        const peerConfig = document.createElement('div');
        const currentIndex = window.FinGuardConfig.peerIndex;
        peerConfig.className = 'peer-config';
        peerConfig.setAttribute('data-peer-index', currentIndex);
        peerConfig.innerHTML = `
            <h5>Peer ${currentIndex + 1} <button type="button" class="btn-danger btn-small" onclick="window.TunnelsManager.removePeer(${currentIndex})">Remove</button></h5>
            <div class="form-group">
                <label>Peer Name</label>
                <input type="text" class="peer-name" placeholder="Server ${currentIndex + 1}">
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
        peerSection.appendChild(peerConfig);
        window.FinGuardConfig.peerIndex++;
    }

    // MARK: initializeForm
    static initializeForm() {
        const form = document.getElementById('tunnelForm');
        if (!form) {
            console.error('Tunnel form not found');
            return;
        }

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            
            TunnelsManager.scrollToTop();
            
            const peers = [];
            const peerConfigs = document.querySelectorAll('.peer-config');
            
            for (let peerConfig of peerConfigs) {
                const publicKey = peerConfig.querySelector('.peer-public-key').value.trim();
                const endpoint = peerConfig.querySelector('.peer-endpoint').value.trim();
                const allowedIPs = peerConfig.querySelector('.peer-allowed-ips').value.trim();
                const keepaliveValue = parseInt(peerConfig.querySelector('.peer-keepalive').value) || 0;
                
                if (!publicKey || !endpoint || !allowedIPs) {
                    window.Utils.showAlert('All peer fields are required', 'error');
                    return;
                }
                
                peers.push({
                    name: peerConfig.querySelector('.peer-name').value.trim() || 'Peer',
                    public_key: publicKey,
                    endpoint: endpoint,
                    allowed_ips: allowedIPs.split(',').map(ip => ip.trim()).filter(ip => ip),
                    persistent_keepalive: keepaliveValue > 0 ? keepaliveValue : 0,
                    preshared_key: peerConfig.querySelector('.peer-preshared').value.trim()
                });
            }
            
            // Get advanced tunnel settings
            const monitorInterval = parseInt(document.getElementById('tunnelMonitorInterval')?.value) || 30;
            const staleTimeout = parseInt(document.getElementById('tunnelStaleTimeout')?.value) || 300;
            const reconnectRetries = parseInt(document.getElementById('tunnelReconnectRetries')?.value) || 3;
            
            const tunnel = {
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
            
            if (!TunnelsManager.validateTunnel(tunnel, peers)) {
                return;
            }
            
            try {
                window.Utils.showAlert('Creating tunnel...', 'info');
                
                await window.APIClient.addTunnel(tunnel);
                
                window.Utils.showAlert(`Tunnel "${tunnel.name}" created successfully! 🎉`, 'success');
                
                TunnelsManager.clearForm();
                TunnelsManager.loadTunnels();
                TunnelsManager.scrollToTop();
                
            } catch (error) {
                console.error('Failed to create tunnel:', error);
                window.Utils.showAlert(`Failed to create tunnel: ${error.message}`, 'error');
                TunnelsManager.scrollToTop();
            }
        });
    }

    static removePeer(index) {
        const peerConfig = document.querySelector(`[data-peer-index="${index}"]`);
        if (peerConfig) {
            peerConfig.remove();
        }
    }

    static scrollToTop() {
        // Smooth scroll to top of page
        window.scrollTo({
            top: 0,
            behavior: 'smooth'
        });
        
        // Also scroll the container to top if it exists
        const container = document.querySelector('.container');
        if (container) {
            container.scrollTop = 0;
        }
    }

    // MARK: clearForm
    static clearForm() {
        const form = document.getElementById('tunnelForm');
        if (form) {
            form.reset();
        }
        
        // Reset specific fields to their default values
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
        
        // Clear all peer configurations except the first one
        const peerSection = document.getElementById('peerSection');
        if (peerSection) {
            const peerConfigs = peerSection.querySelectorAll('.peer-config');
            for (let i = 1; i < peerConfigs.length; i++) {
                peerConfigs[i].remove();
            }
            
            // Reset the first peer config
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
        
        window.FinGuardConfig.peerIndex = 1;
        
        const firstField = document.getElementById('tunnelName');
        if (firstField) {
            firstField.focus();
        }
    }

    static validateTunnel(tunnel, peers) {
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
        
        if (peers.length === 0) {
            window.Utils.showAlert('At least one peer is required', 'error');
            return false;
        }
        
        if (!/^[a-zA-Z0-9-_]+$/.test(tunnel.name)) {
            window.Utils.showAlert('Tunnel name can only contain letters, numbers, hyphens, and underscores', 'error');
            return false;
        }
        
        if (tunnel.private_key.length !== 44 || !/^[A-Za-z0-9+/]+=*$/.test(tunnel.private_key)) {
            window.Utils.showAlert('Invalid private key format. Should be 44 characters base64-encoded.', 'error');
            return false;
        }

        // Validate addresses
        for (const addr of tunnel.addresses) {
            if (!TunnelsManager.isValidCIDR(addr)) {
                window.Utils.showAlert(`Invalid address format: ${addr}. Use CIDR notation (e.g., 10.0.0.1/24)`, 'error');
                return false;
            }
        }

        // Validate peer allowed IPs
        for (const peer of peers) {
            for (const allowedIP of peer.allowed_ips) {
                if (!TunnelsManager.isValidCIDR(allowedIP)) {
                    window.Utils.showAlert(`Invalid allowed IP format for peer ${peer.name}: ${allowedIP}`, 'error');
                    return false;
                }
            }
        }

        return true;
    }

    static isValidCIDR(cidr) {
        // Basic CIDR validation
        const cidrRegex = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;
        if (!cidrRegex.test(cidr)) {
            return false;
        }
        
        const [ip, prefix] = cidr.split('/');
        const octets = ip.split('.');
        
        // Validate IP octets
        for (const octet of octets) {
            const num = parseInt(octet);
            if (num < 0 || num > 255) {
                return false;
            }
        }
        
        // Validate prefix
        const prefixNum = parseInt(prefix);
        if (prefixNum < 0 || prefixNum > 32) {
            return false;
        }
        
        return true;
    }

    static resetForm() {
        TunnelsManager.clearForm();
    }
}

// Export to global scope
window.TunnelsManager = TunnelsManager;