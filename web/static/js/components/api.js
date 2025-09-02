
// API COMMUNICATION LAYER

class APIClient {
    // MARK: apiCall
    static async apiCall(endpoint, options = {}) {
        this.validateAuthentication();
        
        try {
            const response = await this.makeRequest(endpoint, options);
            if (!response.ok) {
                await this.handleResponseErrors(response);
            }
            return await response.json();
        } catch (error) {
            this.handleApiError(error);
            throw error;
        }
    }

    // MARK: validateAuthentication
    static validateAuthentication() {
        if (!window.FinGuardConfig.ADMIN_TOKEN) {
            window.AuthManager.showTokenModal();
            throw new Error('Authentication required');
        }
    }

    // MARK: makeRequest
    static async makeRequest(endpoint, options) {
        return await fetch(`${window.FinGuardConfig.API_BASE}${endpoint}`, {
            ...options,
            headers: {
                'Authorization': `Bearer ${window.FinGuardConfig.ADMIN_TOKEN}`,
                'Content-Type': 'application/json',
                ...options.headers
            }
        });
    }

    // MARK: handleResponseErrors
    static async handleResponseErrors(response) {
        if (response.status === 401) {
            window.AuthManager.clearToken();
            throw new Error('Authentication failed - please re-enter your token');
        }

        let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
        try {
            const data = await response.clone().json();
            if (data && data.error) {
                errorMessage = data.error;
            }
        } catch (e) {
            // ignore JSON parse error
        }

        throw new Error(errorMessage);
    }

    // MARK: handleApiError
    static handleApiError(error) {
        if (!error.message.includes('Authentication')) {
            window.Utils.showAlert(error.message, 'error');
        }
    }

    // SYSTEM ENDPOINTS

    // MARK: restartSystem
    static async restartSystem() {
        return await this.apiCall('/system/restart', {
            method: 'POST'
        });
    }

    // MARK: shutdownSystem
    static async shutdownSystem() {
        return await this.apiCall('/system/shutdown', {
            method: 'POST'
        });
    }

    // MARK: getStatus
    static async getStatus() {
        return await this.apiCall('/status');
    }

    // SERVICE ENDPOINTS

    // MARK: getServices
    static async getServices() {
        return await this.apiCall('/services');
    }

    // MARK: addService
    static async addService(service) {
        return await this.apiCall('/services', {
            method: 'POST',
            body: JSON.stringify(service)
        });
    }

    // MARK: deleteService
    static async deleteService(name) {
        return await this.apiCall(`/services/${encodeURIComponent(name)}`, { 
            method: 'DELETE' 
        });
    }

    // TUNNEL ENDPOINTS

    // MARK: getTunnels
    static async getTunnels() {
        return await this.apiCall('/tunnels');
    }

    // MARK: addTunnel
    static async addTunnel(tunnel) {
        return await this.apiCall('/tunnels', {
            method: 'POST',
            body: JSON.stringify(tunnel)
        });
    }

    // MARK: restartTunnel
    static async restartTunnel(name) {
        return await this.apiCall(`/tunnels/restart/${encodeURIComponent(name)}`, {
            method: 'POST'
        });
    }

    // MARK: deleteTunnel
    static async deleteTunnel(name) {
        return await this.apiCall(`/tunnels/${encodeURIComponent(name)}`, { 
            method: 'DELETE' 
        });
    }

    // LOG ENDPOINTS

    // MARK: getLogs
    static async getLogs(query = '') {
        return await this.apiCall(`/logs${query}`);
    }

    // UPDATE ENDPOINTS

    // MARK: getUpdateStatus
    static async getUpdateStatus() {
        return await this.apiCall('/update/status');
    }

    // MARK: checkForUpdate
    static async checkForUpdate() {
        return await this.apiCall('/update/check', {
            method: 'POST'
        });
    }

    // MARK: applyUpdate
    static async applyUpdate() {
        return await this.apiCall('/update/apply', {
            method: 'POST'
        });
    }

    // MARK: getUpdateConfig
    static async getUpdateConfig() {
        return await this.apiCall('/update/config');
    }

    // MARK: saveUpdateConfig
    static async saveUpdateConfig(config) {
        return await this.apiCall('/update/config', {
            method: 'POST',
            body: JSON.stringify(config)
        });
    }
}

// GLOBAL SCOPE EXPORT

window.APIClient = APIClient;