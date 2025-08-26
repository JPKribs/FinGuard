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

    // Enhanced renderTunnelsList for tunnels.js
static renderTunnelsList(tunnels) {
    const tunnelsList = document.getElementById('tunnelsList');
    
    if (tunnels.length === 0) {
        tunnelsList.innerHTML = '<p style="color: var(--color-text-secondary); text-align: center; padding: 2rem;">No tunnels configured</p>';
        return;
    }
    
    tunnelsList.innerHTML = tunnels.map(tunnel => {
        const details = [];
        
        // Add interface info if available
        if (tunnel.interface) {
            details.push(`Interface: ${tunnel.interface}`);
        }
        
        // Add peer count
        details.push(`Peers: ${tunnel.peers || 0}`);
        
        // Add MTU
        details.push(`MTU: ${tunnel.mtu || 'N/A'}`);
        
        // Add addresses if available (this would need backend support)
        // details.push(`Addresses: ${tunnel.addresses ? tunnel.addresses.join(', ') : 'None'}`);
        
        return `
            <div class="list-item">
                <div>
                    <strong>${window.Utils.escapeHtml(tunnel.name)}</strong><br>
                    <small>${details.join(' | ')}</small>
                </div>
                <div class="actions">
                    <span class="status ${tunnel.state === 'running' ? 'running' : 'stopped'}">${tunnel.state}</span>
                    <button class="btn-danger" onclick="window.TunnelsManager.deleteTunnel('${window.Utils.escapeHtml(tunnel.name)}')">Delete</button>
                </div>
            </div>
        `;
    }).join('');
}

    static async deleteTunnel(name) {
        if (!confirm(`Delete tunnel "${name}"? This will also remove any services using this tunnel.`)) return;
        
        try {
            await window.APIClient.deleteTunnel(name);
            window.Utils.showAlert(`Tunnel "${name}" deleted successfully`);
            TunnelsManager.loadTunnels();
            window.ServicesManager.loadServices();
        } catch (error) {
            console.error('Failed to delete tunnel:', error);
        }
    }

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
                <input type="number" class="peer-keepalive" placeholder="25" value="25">
            </div>
            <div class="form-group">
                <label>Preshared Key (optional)</label>
                <textarea class="peer-preshared" placeholder="Optional preshared key" rows="2"></textarea>
            </div>
        `;
        peerSection.appendChild(peerConfig);
        window.FinGuardConfig.peerIndex++;
    }

    static removePeer(index) {
        const peerConfig = document.querySelector(`[data-peer-index="${index}"]`);
        if (peerConfig) {
            peerConfig.remove();
        }
    }

    static initializeForm() {
        document.getElementById('tunnelForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            
            const peers = [];
            const peerConfigs = document.querySelectorAll('.peer-config');
            
            for (let peerConfig of peerConfigs) {
                const publicKey = peerConfig.querySelector('.peer-public-key').value.trim();
                const endpoint = peerConfig.querySelector('.peer-endpoint').value.trim();
                const allowedIPs = peerConfig.querySelector('.peer-allowed-ips').value.trim();
                
                if (!publicKey || !endpoint || !allowedIPs) {
                    window.Utils.showAlert('All peer fields are required', 'error');
                    return;
                }
                
                peers.push({
                    name: peerConfig.querySelector('.peer-name').value.trim() || 'Peer',
                    public_key: publicKey,
                    endpoint: endpoint,
                    allowed_ips: allowedIPs.split(',').map(ip => ip.trim()).filter(ip => ip),
                    persistent_keepalive: parseInt(peerConfig.querySelector('.peer-keepalive').value) || 0,
                    preshared_key: peerConfig.querySelector('.peer-preshared').value.trim()
                });
            }
            
            // Parse routes from form input
            const routesInput = document.getElementById('tunnelRoutes').value.trim();
            const routes = routesInput ? routesInput.split(',').map(route => route.trim()).filter(route => route) : [];
            
            const tunnel = {
                name: document.getElementById('tunnelName').value.trim(),
                listen_port: parseInt(document.getElementById('tunnelListenPort').value) || 0,
                private_key: document.getElementById('tunnelPrivateKey').value.trim(),
                mtu: parseInt(document.getElementById('tunnelMTU').value) || 1420,
                addresses: [document.getElementById('tunnelAddress').value.trim()].filter(addr => addr),
                routes: routes, // Add routes to tunnel creation
                peers: peers
            };
            
            if (!TunnelsManager.validateTunnel(tunnel, peers)) {
                return;
            }
            
            try {
                await window.APIClient.addTunnel(tunnel);
                window.Utils.showAlert(`Tunnel "${tunnel.name}" created successfully`);
                TunnelsManager.resetForm();
                TunnelsManager.loadTunnels();
            } catch (error) {
                console.error('Failed to create tunnel:', error);
            }
        });
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
        
        if (!/^[a-zA-Z0-9-]+$/.test(tunnel.name)) {
            window.Utils.showAlert('Tunnel name can only contain letters, numbers, and hyphens', 'error');
            return false;
        }
        
        if (tunnel.private_key.length !== 44 || !/^[A-Za-z0-9+/]+=*$/.test(tunnel.private_key)) {
            window.Utils.showAlert('Invalid private key format. Should be 44 characters base64-encoded.', 'error');
            return false;
        }

        return true;
    }

    static resetForm() {
        document.getElementById('tunnelForm').reset();
        document.getElementById('tunnelMTU').value = '1420';
        
        document.getElementById('peerSection').innerHTML = `
            <div class="peer-config" data-peer-index="0">
                <div class="form-group">
                    <label>Peer Name</label>
                    <input type="text" class="peer-name" placeholder="Server">
                </div>
                <div class="form-group">
                    <label>Public Key</label>
                    <textarea class="peer-public-key" placeholder="WGqWDWQ3aQLK6tD7dlKud4PCK515UpopGgAGc2f7FS4=" rows="2" required></textarea>
                </div>
                <div class="form-group">
                    <label>Endpoint</label>
                    <input type="text" class="peer-endpoint" placeholder="kelp.ink:6806" required>
                </div>
                <div class="form-group">
                    <label>Allowed IPs (comma-separated)</label>
                    <input type="text" class="peer-allowed-ips" placeholder="10.20.1.0/24" required>
                </div>
                <div class="form-group">
                    <label>Persistent Keepalive (seconds)</label>
                    <input type="number" class="peer-keepalive" placeholder="25" value="25">
                </div>
                <div class="form-group">
                    <label>Preshared Key (optional)</label>
                    <textarea class="peer-preshared" placeholder="Optional preshared key" rows="2"></textarea>
                </div>
            </div>
        `;
        window.FinGuardConfig.peerIndex = 1;
    }
}

// Export to global scope
window.TunnelsManager = TunnelsManager;