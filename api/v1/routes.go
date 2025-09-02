package v1

import (
	"net/http"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/mdns"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/updater"
	"github.com/JPKribs/FinGuard/wireguard"
)

// MARK: NewAPIServer
// Create a new instance of the API Server.
func NewAPIServer(cfg *config.Config, proxyServer *proxy.Server, tunnelManager wireguard.TunnelManager, discoveryManager *mdns.Discovery, logger *internal.Logger, updateManager *updater.UpdateManager) *APIServer {
	return &APIServer{
		cfg:              cfg,
		proxyServer:      proxyServer,
		tunnelManager:    tunnelManager,
		discoveryManager: discoveryManager,
		logger:           logger,
		updateManager:    updateManager,
	}
}

// MARK: RegisterRoutes
// Register all API Routes.
func (a *APIServer) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))
	mux.HandleFunc("/", a.handleWebUI)
	mux.HandleFunc("/api/v1/services", a.authMiddleware(a.handleServices))
	mux.HandleFunc("/api/v1/services/", a.authMiddleware(a.handleServiceByName))
	mux.HandleFunc("/api/v1/tunnels", a.authMiddleware(a.handleTunnels))
	mux.HandleFunc("/api/v1/tunnels/", a.authMiddleware(a.handleTunnelByName))
	mux.HandleFunc("/api/v1/tunnels/restart/", a.authMiddleware(a.handleTunnelRestart))
	mux.HandleFunc("/api/v1/system/restart", a.authMiddleware(a.handleSystemRestart))
	mux.HandleFunc("/api/v1/system/shutdown", a.authMiddleware(a.handleSystemShutdown))
	mux.HandleFunc("/api/v1/status", a.authMiddleware(a.handleStatus))
	mux.HandleFunc("/api/v1/logs", a.authMiddleware(a.handleLogs))
	mux.HandleFunc("/api/v1/update/status", a.authMiddleware(a.handleUpdateStatus))
	mux.HandleFunc("/api/v1/update/check", a.authMiddleware(a.handleUpdateCheck))
	mux.HandleFunc("/api/v1/update/apply", a.authMiddleware(a.handleUpdateApply))
	mux.HandleFunc("/api/v1/update/config", a.authMiddleware(a.handleUpdateConfig))
}
