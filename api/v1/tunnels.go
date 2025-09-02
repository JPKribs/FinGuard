package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: handleTunnels
func (a *APIServer) handleTunnels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		a.handleListTunnels(w, r, ctx)
	case http.MethodPost:
		a.handleCreateTunnel(w, r)
	default:
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// MARK: handleTunnelByName
func (a *APIServer) handleTunnelByName(w http.ResponseWriter, r *http.Request) {
	tunnelName := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/")
	if tunnelName == "" {
		a.respondWithError(w, http.StatusBadRequest, "Tunnel name required")
		return
	}

	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		a.handleGetTunnel(w, r, ctx, tunnelName)
	case http.MethodDelete:
		a.handleDeleteTunnel(w, r, ctx, tunnelName)
	default:
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// MARK: handleListTunnels
func (a *APIServer) handleListTunnels(w http.ResponseWriter, r *http.Request, ctx context.Context) {
	configuredTunnels := a.cfg.WireGuard.Tunnels
	runningTunnels, err := a.tunnelManager.ListTunnels(ctx)
	if err != nil {
		a.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	runningMap := make(map[string]TunnelStatus)
	for _, tunnel := range runningTunnels {
		runningMap[tunnel.Name] = tunnel
	}

	tunnelStatuses := make([]TunnelStatus, 0, len(configuredTunnels))
	for _, configTunnel := range configuredTunnels {
		if runningTunnel, exists := runningMap[configTunnel.Name]; exists {
			tunnelStatuses = append(tunnelStatuses, runningTunnel)
		} else {
			tunnelStatuses = append(tunnelStatuses, TunnelStatus{
				Name:      configTunnel.Name,
				State:     "stopped",
				Interface: "",
				MTU:       configTunnel.MTU,
				Peers:     len(configTunnel.Peers),
			})
		}
	}

	a.respondWithSuccess(w, "Tunnels retrieved", tunnelStatuses)
}

// MARK: handleCreateTunnel
func (a *APIServer) handleCreateTunnel(w http.ResponseWriter, r *http.Request) {
	var req TunnelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	tunnelConfig := a.convertTunnelRequest(req)

	if err := a.cfg.AddTunnel(tunnelConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	if err := a.tunnelManager.CreateTunnel(ctx, tunnelConfig); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := TunnelStatus{
		Name:      tunnelConfig.Name,
		State:     "running",
		Interface: "",
		MTU:       tunnelConfig.MTU,
		Peers:     len(tunnelConfig.Peers),
	}

	a.respondWithSuccess(w, "Tunnel created", response)
}

// MARK: handleGetTunnel
func (a *APIServer) handleGetTunnel(w http.ResponseWriter, r *http.Request, ctx context.Context, tunnelName string) {
	configTunnel := a.cfg.GetTunnel(tunnelName)
	if configTunnel == nil {
		a.respondWithError(w, http.StatusNotFound, "Tunnel not found")
		return
	}

	status, err := a.tunnelManager.Status(ctx, tunnelName)
	if err != nil {
		status = TunnelStatus{
			Name:      configTunnel.Name,
			State:     "stopped",
			Interface: "",
			MTU:       configTunnel.MTU,
			Peers:     len(configTunnel.Peers),
		}
	}

	a.respondWithSuccess(w, "Tunnel retrieved", status)
}

// MARK: handleDeleteTunnel
func (a *APIServer) handleDeleteTunnel(w http.ResponseWriter, r *http.Request, ctx context.Context, tunnelName string) {
	if err := a.tunnelManager.DeleteTunnel(ctx, tunnelName); err != nil {
		a.logger.Warn("Failed to stop tunnel", "tunnel", tunnelName, "error", err)
	}

	if err := a.cfg.RemoveTunnel(tunnelName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Tunnel not found")
		return
	}

	a.respondWithSuccess(w, "Tunnel deleted", nil)
}

// MARK: handleTunnelRestart
func (a *APIServer) handleTunnelRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	tunnelName := strings.TrimPrefix(r.URL.Path, "/api/v1/tunnels/restart/")
	if tunnelName == "" {
		a.respondWithError(w, http.StatusBadRequest, "Tunnel name required")
		return
	}

	tunnelConfig := a.cfg.GetTunnel(tunnelName)
	if tunnelConfig == nil {
		a.respondWithError(w, http.StatusNotFound, "Tunnel not found")
		return
	}

	ctx := r.Context()

	if err := a.tunnelManager.DeleteTunnel(ctx, tunnelName); err != nil {
		a.logger.Warn("Failed to stop tunnel during restart", "tunnel", tunnelName, "error", err)
	}

	if err := a.tunnelManager.CreateTunnel(ctx, *tunnelConfig); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, "Failed to restart tunnel: "+err.Error())
		return
	}

	status, _ := a.tunnelManager.Status(ctx, tunnelName)
	a.respondWithSuccess(w, "Tunnel restarted", status)
}

// MARK: convertTunnelRequest
func (a *APIServer) convertTunnelRequest(req TunnelCreateRequest) config.TunnelConfig {
	if req.MTU == 0 {
		req.MTU = 1420
	}
	if req.MonitorInterval == 0 {
		req.MonitorInterval = 25
	}
	if req.StaleConnectionTimeout == 0 {
		req.StaleConnectionTimeout = 300
	}
	if req.ReconnectionRetries == 0 {
		req.ReconnectionRetries = 3
	}

	peers := make([]config.PeerConfig, len(req.Peers))
	for i, peerReq := range req.Peers {
		peers[i] = config.PeerConfig{
			Name:                   peerReq.Name,
			PublicKey:              peerReq.PublicKey,
			AllowedIPs:             peerReq.AllowedIPs,
			Endpoint:               peerReq.Endpoint,
			Preshared:              peerReq.PresharedKey,
			Persistent:             peerReq.PersistentKeepalive > 0,
			PersistentKeepaliveInt: peerReq.PersistentKeepalive,
		}
	}

	return config.TunnelConfig{
		Name:                   req.Name,
		ListenPort:             req.ListenPort,
		PrivateKey:             req.PrivateKey,
		MTU:                    req.MTU,
		Addresses:              req.Addresses,
		Routes:                 req.Routes,
		Peers:                  peers,
		MonitorInterval:        req.MonitorInterval,
		StaleConnectionTimeout: req.StaleConnectionTimeout,
		ReconnectionRetries:    req.ReconnectionRetries,
	}
}
