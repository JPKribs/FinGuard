
// AUTHENTICATION MANAGEMENT

class AuthManager {
    // MARK: getConfig
    static getConfig() {
        if (!window.FinGuardConfig) {
            console.error('FinGuardConfig not available yet');
            return { API_BASE: '/api/v1' };
        }
        return window.FinGuardConfig;
    }

    // TOKEN MODAL MANAGEMENT

    // MARK: showTokenModal
    static showTokenModal() {
        this.hideTokenModal();
        
        const modal = this.createTokenModal();
        document.body.appendChild(modal);
        
        this.setupTokenForm();
        this.focusTokenInput();
    }

    // MARK: createTokenModal
    static createTokenModal() {
        const modal = document.createElement('div');
        modal.id = 'tokenModal';
        modal.className = 'token-modal';
        modal.innerHTML = this.getTokenModalHTML();
        return modal;
    }

    // MARK: getTokenModalHTML
    static getTokenModalHTML() {
        return `
            <div class="token-modal-content">
                <h3>Admin Authentication</h3>
                <p>Enter your Admin Token</p>
                <form id="tokenForm">
                    <div class="form-group">
                        <input type="password" id="tokenInput" placeholder="Admin token" required>
                        <div class="token-actions">
                            <button type="submit">Authenticate</button>
                            <button type="button" onclick="window.AuthManager.clearStoredToken()">Clear Stored Token</button>
                        </div>
                    </div>
                </form>
                <div id="tokenError" class="token-error hidden"></div>
            </div>
        `;
    }

    // MARK: setupTokenForm
    static setupTokenForm() {
        const tokenForm = document.getElementById('tokenForm');
        tokenForm.addEventListener('submit', this.handleTokenSubmit.bind(this));
    }

    // MARK: handleTokenSubmit
    static async handleTokenSubmit(e) {
        e.preventDefault();
        const token = document.getElementById('tokenInput').value.trim();
        
        if (!token) {
            this.showTokenError('Please enter a token');
            return;
        }
        
        await this.verifyToken(token);
    }

    // MARK: focusTokenInput
    static focusTokenInput() {
        const tokenInput = document.getElementById('tokenInput');
        if (tokenInput) {
            tokenInput.focus();
        }
    }

    // MARK: hideTokenModal
    static hideTokenModal() {
        const existingModals = document.querySelectorAll('#tokenModal');
        existingModals.forEach(modal => modal.remove());
    }

    // ERROR HANDLING

    // MARK: showTokenError
    static showTokenError(message) {
        const errorDiv = document.getElementById('tokenError');
        if (!errorDiv) return;
        
        errorDiv.textContent = message;
        errorDiv.classList.remove('hidden');
        
        setTimeout(() => {
            errorDiv.classList.add('hidden');
        }, 5000);
    }

    // TOKEN VERIFICATION

    // MARK: verifyToken
    static async verifyToken(token) {
        try {
            const response = await this.makeAuthRequest(token);
            
            if (response.ok) {
                this.handleSuccessfulAuth(token);
            } else {
                await this.handleFailedAuth(response);
            }
        } catch (error) {
            this.handleNetworkError(error);
        }
    }

    // MARK: makeAuthRequest
    static async makeAuthRequest(token) {
        const config = this.getConfig();
        return await fetch(`${config.API_BASE}/status`, {
            headers: {
                'Authorization': `Bearer ${token}`,
                'Content-Type': 'application/json'
            }
        });
    }

    // MARK: handleSuccessfulAuth
    static handleSuccessfulAuth(token) {
        this.setAuthToken(token);
        this.hideTokenModal();
        this.showSuccessMessage();
        this.initializeApp();
    }

    // MARK: handleFailedAuth
    static async handleFailedAuth(response) {
        const error = await response.json().catch(() => ({ error: 'Invalid token' }));
        this.showTokenError(error.error || 'Authentication failed');
        this.clearTokenInput();
        this.focusTokenInput();
    }

    // MARK: handleNetworkError
    static handleNetworkError(error) {
        this.showTokenError('Network error: ' + error.message);
        this.focusTokenInput();
    }

    // TOKEN MANAGEMENT

    // MARK: setAuthToken
    static setAuthToken(token) {
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = token;
        }
    }

    // MARK: clearTokenInput
    static clearTokenInput() {
        const tokenInput = document.getElementById('tokenInput');
        if (tokenInput) {
            tokenInput.value = '';
        }
    }

    // MARK: showSuccessMessage
    static showSuccessMessage() {
        if (window.Utils) {
            window.Utils.showAlert('Authentication successful!', 'success');
        }
    }

    // MARK: initializeApp
    static initializeApp() {
        if (window.App) {
            window.App.initializeApp();
        }
    }

    // MARK: clearToken
    static clearToken() {
        this.removeStoredToken();
        this.resetAuthConfig();
        this.hideTokenModal();
        
        setTimeout(() => {
            this.showTokenModal();
        }, 200);
    }

    // MARK: clearStoredToken
    static clearStoredToken() {
        this.removeStoredToken();
        this.resetAuthConfig();
        this.hideTokenModal();
        
        setTimeout(() => {
            this.showTokenModal();
        }, 100);
    }

    // MARK: removeStoredToken
    static removeStoredToken() {
        localStorage.removeItem('adminToken');
    }

    // MARK: resetAuthConfig
    static resetAuthConfig() {
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = null;
        }
    }
}

// GLOBAL SCOPE EXPORT

window.AuthManager = AuthManager;