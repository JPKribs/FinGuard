// Configuration and Global State
(function() {
    const API_BASE = '/api/v1';
    let ADMIN_TOKEN = localStorage.getItem('adminToken');
    let peerIndex = 1;

    // Create global configuration object immediately
    window.FinGuardConfig = {
        API_BASE,
        get ADMIN_TOKEN() { return ADMIN_TOKEN; },
        set ADMIN_TOKEN(token) { 
            ADMIN_TOKEN = token; 
            if (token) {
                localStorage.setItem('adminToken', token);
            } else {
                localStorage.removeItem('adminToken');
            }
        },
        get peerIndex() { return peerIndex; },
        set peerIndex(index) { peerIndex = index; }
    };

    // Ensure config is available
    console.log('FinGuard config initialized:', window.FinGuardConfig);
})();