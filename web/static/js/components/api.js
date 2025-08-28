// API Communication Layer
class APIClient {
    static async apiCall(endpoint, options = {}) {
        if (!window.FinGuardConfig.ADMIN_TOKEN) {
            window.AuthManager.showTokenModal();
            throw new Error('Authentication required');
        }
        
        try {
            const response = await fetch(`${window.FinGuardConfig.API_BASE}${endpoint}`, {
                ...options,
                headers: {
                    'Authorization': `Bearer ${window.FinGuardConfig.ADMIN_TOKEN}`,
                    'Content-Type': 'application/json',
                    ...options.headers
                }
            });
            
            if (response.status === 401) {
                window.AuthManager.clearToken();
                throw new Error('Authentication failed - please re-enter your token');
            }
            
            if (!response.ok) {
                const error = await response.json().catch(() => ({ 
                    error: `HTTP ${response.status}: ${response.statusText}` 
                }));
                throw new Error(error.error || `HTTP ${response.status}: ${response.statusText}`);
            }
            
            return await response.json();
        } catch (error) {
            if (error.message.includes('Authentication')) {
                throw error;
            }
            window.Utils.showAlert(error.message, 'error');
            throw error;
        }
    }

    static async restartSystem() {
        return await APIClient.apiCall('/system/restart', {
            method: 'POST'
        });
    }

    static async shutdownSystem() {
        return await APIClient.apiCall('/system/shutdown', {
            method: 'POST'
        });
    }

    static async getServices() {
        return await APIClient.apiCall('/services');
    }

    static async addService(service) {
        return await APIClient.apiCall('/services', {
            method: 'POST',
            body: JSON.stringify(service)
        });
    }

    static async deleteService(name) {
        return await APIClient.apiCall(`/services/${encodeURIComponent(name)}`, { 
            method: 'DELETE' 
        });
    }

    static async getTunnels() {
        return await APIClient.apiCall('/tunnels');
    }

    static async addTunnel(tunnel) {
        return await APIClient.apiCall('/tunnels', {
            method: 'POST',
            body: JSON.stringify(tunnel)
        });
    }

    static async restartTunnel(name) {
        return await APIClient.apiCall(`/tunnels/restart/${encodeURIComponent(name)}`, {
            method: 'POST'
        });
    }

    static async deleteTunnel(name) {
        return await APIClient.apiCall(`/tunnels/${encodeURIComponent(name)}`, { 
            method: 'DELETE' 
        });
    }

    static async getStatus() {
        return await APIClient.apiCall('/status');
    }

    static async getLogs(query = '') {
        return await APIClient.apiCall(`/logs${query}`);
    }

        static async getUpdateStatus() {
        return await APIClient.apiCall('/update/status');
    }

    static async checkForUpdate() {
        return await APIClient.apiCall('/update/check', {
            method: 'POST'
        });
    }

    static async applyUpdate() {
        return await APIClient.apiCall('/update/apply', {
            method: 'POST'
        });
    }

    static async getUpdateConfig() {
        return await APIClient.apiCall('/update/config');
    }

    static async saveUpdateConfig(config) {
        return await APIClient.apiCall('/update/config', {
            method: 'POST',
            body: JSON.stringify(config)
        });
    }
}

// Export to global scope
window.APIClient = APIClient;