// Authentication Management
class AuthManager {
    static getConfig() {
        // Safety check for config availability
        if (!window.FinGuardConfig) {
            console.error('FinGuardConfig not available yet');
            return { API_BASE: '/api/v1' }; // fallback
        }
        return window.FinGuardConfig;
    }

    static showTokenModal() {
        // Remove any existing modal first
        AuthManager.hideTokenModal();
        
        const modal = document.createElement('div');
        modal.id = 'tokenModal';
        modal.className = 'token-modal';
        modal.innerHTML = `
            <div class="token-modal-content">
                <h3>Admin Authentication</h3>
                <p>Enter your admin token to access the FinGuard management interface:</p>
                <form id="tokenForm">
                    <div class="form-group">
                        <input type="password" id="tokenInput" placeholder="Enter admin token" required>
                        <div class="token-actions">
                            <button type="submit">Authenticate</button>
                            <button type="button" onclick="window.AuthManager.clearStoredToken()">Clear Stored Token</button>
                        </div>
                    </div>
                </form>
                <div id="tokenError" class="token-error hidden"></div>
            </div>
        `;
        
        document.body.appendChild(modal);
        document.getElementById('tokenInput').focus();
        
        document.getElementById('tokenForm').addEventListener('submit', async (e) => {
            e.preventDefault();
            const token = document.getElementById('tokenInput').value.trim();
            
            if (!token) {
                AuthManager.showTokenError('Please enter a token');
                return;
            }
            
            await AuthManager.verifyToken(token);
        });
    }

    static hideTokenModal() {
        // Remove all existing modals
        const existingModals = document.querySelectorAll('#tokenModal');
        existingModals.forEach(modal => {
            modal.remove();
        });
    }

    static showTokenError(message) {
        const errorDiv = document.getElementById('tokenError');
        if (errorDiv) {
            errorDiv.textContent = message;
            errorDiv.classList.remove('hidden');
            
            setTimeout(() => {
                errorDiv.classList.add('hidden');
            }, 5000);
        }
    }

    static async verifyToken(token) {
        try {
            const config = AuthManager.getConfig();
            const response = await fetch(`${config.API_BASE}/status`, {
                headers: {
                    'Authorization': `Bearer ${token}`,
                    'Content-Type': 'application/json'
                }
            });
            
            if (response.ok) {
                if (window.FinGuardConfig) {
                    window.FinGuardConfig.ADMIN_TOKEN = token;
                }
                AuthManager.hideTokenModal();
                if (window.Utils) {
                    window.Utils.showAlert('Authentication successful!', 'success');
                }
                if (window.App) {
                    window.App.initializeApp();
                }
            } else {
                const error = await response.json().catch(() => ({ error: 'Invalid token' }));
                AuthManager.showTokenError(error.error || 'Authentication failed');
                document.getElementById('tokenInput').value = '';
                document.getElementById('tokenInput').focus();
            }
        } catch (error) {
            AuthManager.showTokenError('Network error: ' + error.message);
            const tokenInput = document.getElementById('tokenInput');
            if (tokenInput) {
                tokenInput.focus();
            }
        }
    }

    static clearToken() {
        localStorage.removeItem('adminToken');
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = null;
        }
        AuthManager.hideTokenModal();
        // Small delay to ensure cleanup is complete
        setTimeout(() => {
            AuthManager.showTokenModal();
        }, 200);
    }

    static clearStoredToken() {
        localStorage.removeItem('adminToken');
        if (window.FinGuardConfig) {
            window.FinGuardConfig.ADMIN_TOKEN = null;
        }
        AuthManager.hideTokenModal();
        setTimeout(() => {
            AuthManager.showTokenModal();
        }, 100);
    }
}

// Export to global scope
window.AuthManager = AuthManager;