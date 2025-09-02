
// CONFIGURATION AND GLOBAL STATE MANAGEMENT

class ConfigManager {
    constructor() {
        this.API_BASE = '/api/v1';
        this.adminToken = this.loadStoredToken();
        this.peerIndex = 1;
        
        this.initializeGlobalConfig();
        this.logInitialization();
    }

    // TOKEN MANAGEMENT

    // MARK: loadStoredToken
    loadStoredToken() {
        return localStorage.getItem('adminToken');
    }

    // MARK: setAdminToken
    setAdminToken(token) {
        this.adminToken = token;
        
        if (token) {
            localStorage.setItem('adminToken', token);
        } else {
            localStorage.removeItem('adminToken');
        }
    }

    // MARK: getAdminToken
    getAdminToken() {
        return this.adminToken;
    }

    // PEER INDEX MANAGEMENT

    // MARK: getPeerIndex
    getPeerIndex() {
        return this.peerIndex;
    }

    // MARK: setPeerIndex
    setPeerIndex(index) {
        this.peerIndex = index;
    }

    // MARK: incrementPeerIndex
    incrementPeerIndex() {
        this.peerIndex++;
        return this.peerIndex;
    }

    // CONFIGURATION SETUP

    // MARK: initializeGlobalConfig
    initializeGlobalConfig() {
        window.FinGuardConfig = this.createConfigObject();
    }

    // MARK: createConfigObject
    createConfigObject() {
        return {
            API_BASE: this.API_BASE,
            get ADMIN_TOKEN() { 
                return configInstance.getAdminToken(); 
            },
            set ADMIN_TOKEN(token) { 
                configInstance.setAdminToken(token); 
            },
            get peerIndex() { 
                return configInstance.getPeerIndex(); 
            },
            set peerIndex(index) { 
                configInstance.setPeerIndex(index); 
            }
        };
    }

    // MARK: logInitialization
    logInitialization() {
        console.log('FinGuard config initialized:', window.FinGuardConfig);
    }
}

// GLOBAL INITIALIZATION

// Create singleton instance
const configInstance = new ConfigManager();

// Export for potential external access
window.ConfigManager = ConfigManager;