package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/discovery"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/utilities"
	"github.com/JPKribs/FinGuard/wireguard"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type ServiceStatusResponse struct {
	Name        string `json:"name"`
	Upstream    string `json:"upstream"`
	Status      string `json:"status"`
	Tunnel      string `json:"tunnel,omitempty"`
	Websocket   bool   `json:"websocket"`
	Default     bool   `json:"default"`
	PublishMDNS bool   `json:"publish_mdns"`
}

type TunnelStatus = wireguard.TunnelStatus

type TunnelCreateRequest struct {
	Name       string              `json:"name"`
	ListenPort int                 `json:"listen_port"`
	PrivateKey string              `json:"private_key"`
	MTU        int                 `json:"mtu"`
	Addresses  []string            `json:"addresses"`
	Routes     []string            `json:"routes"`
	Peers      []PeerCreateRequest `json:"peers"`
}

type PeerCreateRequest struct {
	Name                string   `json:"name"`
	PublicKey           string   `json:"public_key"`
	AllowedIPs          []string `json:"allowed_ips"`
	Endpoint            string   `json:"endpoint"`
	PresharedKey        string   `json:"preshared_key"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
}

type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

type LogResponse struct {
	Logs   []LogEntry `json:"logs"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type APIServer struct {
	cfg              *config.Config
	proxyServer      *proxy.Server
	tunnelManager    wireguard.TunnelManager
	discoveryManager *discovery.Discovery
	logger           *internal.Logger
}

// MARK: NewAPIServer

// Creates a new API server instance with all dependencies
func NewAPIServer(cfg *config.Config, proxyServer *proxy.Server, tunnelManager wireguard.TunnelManager, discoveryManager *discovery.Discovery, logger *internal.Logger) *APIServer {
	return &APIServer{
		cfg:              cfg,
		proxyServer:      proxyServer,
		tunnelManager:    tunnelManager,
		discoveryManager: discoveryManager,
		logger:           logger,
	}
}

// MARK: RegisterRoutes

// Registers all API endpoints with the HTTP multiplexer
func (a *APIServer) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./web/static/"))))
	mux.HandleFunc("/", a.handleWebUI)
	mux.HandleFunc("/api/v1/services", a.authMiddleware(a.handleServices))
	mux.HandleFunc("/api/v1/services/", a.authMiddleware(a.handleServiceByName))
	mux.HandleFunc("/api/v1/tunnels", a.authMiddleware(a.handleTunnels))
	mux.HandleFunc("/api/v1/tunnels/", a.authMiddleware(a.handleTunnelByName))
	mux.HandleFunc("/api/v1/status", a.authMiddleware(a.handleStatus))
	mux.HandleFunc("/api/v1/logs", a.authMiddleware(a.handleLogs))
}

// MARK: handleWebUI

// Serves the web management interface
func (a *APIServer) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "./web/index.html")
}

// MARK: authMiddleware

// Validates admin token authentication for protected endpoints
func (a *APIServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := a.extractToken(r)
		expectedToken := "Bearer " + a.cfg.Server.AdminToken

		if token != expectedToken {
			a.respondWithError(w, http.StatusUnauthorized, "Invalid or missing admin token")
			return
		}

		next(w, r)
	}
}

// MARK: extractToken

// Extracts authentication token from request headers or query parameters
func (a *APIServer) extractToken(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	if token != "" && !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}

	return token
}

// MARK: handleServices

// Routes service requests to appropriate handlers based on HTTP method
func (a *APIServer) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleListServices(w, r)
	case http.MethodPost:
		a.handleAddService(w, r)
	default:
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// MARK: handleServiceByName

// Routes individual service requests based on HTTP method
func (a *APIServer) handleServiceByName(w http.ResponseWriter, r *http.Request) {
	serviceName := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	if serviceName == "" {
		a.respondWithError(w, http.StatusBadRequest, "Service name required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.handleGetService(w, r, serviceName)
	case http.MethodDelete:
		a.handleDeleteService(w, r, serviceName)
	default:
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// MARK: handleListServices

// Returns list of all configured services with their status
func (a *APIServer) handleListServices(w http.ResponseWriter, r *http.Request) {
	services := a.proxyServer.ListServices()
	statusList := make([]ServiceStatusResponse, 0, len(services))

	for _, svc := range services {
		status := "unknown"
		if _, err := a.proxyServer.GetServiceStatus(svc.Name); err == nil {
			status = "running"
		}

		statusList = append(statusList, ServiceStatusResponse{
			Name:        svc.Name,
			Upstream:    svc.Upstream,
			Status:      status,
			Tunnel:      svc.Tunnel,
			Websocket:   svc.Websocket,
			Default:     svc.Default,
			PublishMDNS: svc.PublishMDNS,
		})
	}

	a.respondWithSuccess(w, "Services retrieved", statusList)
}

// MARK: handleAddService
func (a *APIServer) handleAddService(w http.ResponseWriter, r *http.Request) {
	var svc config.ServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if err := a.validateServiceConfig(svc); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add route to tunnel if service is connected to one
	if svc.Tunnel != "" {
		if err := a.addServiceRouteToTunnel(svc); err != nil {
			a.respondWithError(w, http.StatusInternalServerError, "Failed to add route to tunnel: "+err.Error())
			return
		}
	}

	if err := a.cfg.AddService(svc); err != nil {
		// Rollback route if service creation fails
		if svc.Tunnel != "" {
			a.removeServiceRouteFromTunnel(svc)
		}
		a.respondWithError(w, http.StatusInternalServerError, "Failed to save service: "+err.Error())
		return
	}

	if err := a.proxyServer.AddService(svc); err != nil {
		// Rollback both config and route if proxy fails
		a.cfg.RemoveService(svc.Name)
		if svc.Tunnel != "" {
			a.removeServiceRouteFromTunnel(svc)
		}
		a.respondWithError(w, http.StatusInternalServerError, "Failed to add service: "+err.Error())
		return
	}

	a.publishServiceMDNS(svc)

	// Enhanced success message
	successMessage := fmt.Sprintf("Service %s added successfully", svc.Name)
	if svc.Tunnel != "" {
		successMessage += fmt.Sprintf(" with route to tunnel %s", svc.Tunnel)
	}

	a.respondWithSuccess(w, successMessage, svc)
}

// MARK: handleDeleteService
func (a *APIServer) handleDeleteService(w http.ResponseWriter, r *http.Request, serviceName string) {
	// Get service config before deletion to know which tunnel to update
	var serviceToDelete *config.ServiceConfig
	for _, svc := range a.cfg.Services {
		if svc.Name == serviceName {
			serviceToDelete = &svc
			break
		}
	}

	if serviceToDelete == nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found")
		return
	}

	// Remove route from tunnel if service was connected to one
	if serviceToDelete.Tunnel != "" {
		if err := a.removeServiceRouteFromTunnel(*serviceToDelete); err != nil {
			a.logger.Error("Failed to remove route from tunnel", "error", err)
			// Don't fail deletion, but log the error
		}
	}

	if a.cfg.Discovery.Enable && a.cfg.Discovery.MDNS.Enabled && a.discoveryManager != nil {
		a.discoveryManager.UnpublishService(serviceName)
	}

	if err := a.cfg.RemoveService(serviceName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found in config")
		return
	}

	if err := a.proxyServer.RemoveService(serviceName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found in proxy")
		return
	}

	a.respondWithSuccess(w, fmt.Sprintf("Service %s deleted successfully", serviceName), nil)
}

// MARK: addServiceRouteToTunnel
func (a *APIServer) addServiceRouteToTunnel(svc config.ServiceConfig) error {
	// Extract IP from upstream URL
	serviceIP, err := a.extractIPFromUpstream(svc.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream %s: %w", svc.Upstream, err)
	}

	// Get the tunnel configuration
	tunnel := a.cfg.GetTunnel(svc.Tunnel)
	if tunnel == nil {
		return fmt.Errorf("tunnel %s not found", svc.Tunnel)
	}

	// Create route in CIDR format (assuming /32 for individual IP)
	route := serviceIP + "/32"

	// Check if route already exists
	for _, existingRoute := range tunnel.Routes {
		if existingRoute == route {
			a.logger.Info("Route already exists for service", "service", svc.Name, "route", route)
			return nil
		}
	}

	// Add route to tunnel configuration
	tunnel.Routes = append(tunnel.Routes, route)

	// Update tunnel in config
	if err := a.cfg.UpdateTunnel(*tunnel); err != nil {
		return fmt.Errorf("failed to update tunnel config: %w", err)
	}

	// Update the running tunnel if it exists
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.tunnelManager.UpdateTunnel(ctx, *tunnel); err != nil {
		// Log warning but don't fail - tunnel might not be running
		a.logger.Warn("Failed to update running tunnel, route will be added on next start",
			"tunnel", svc.Tunnel, "route", route, "error", err)
	}

	a.logger.Info("Added service route to tunnel",
		"service", svc.Name, "tunnel", svc.Tunnel, "route", route)

	return nil
}

// MARK: removeServiceRouteFromTunnel
func (a *APIServer) removeServiceRouteFromTunnel(svc config.ServiceConfig) error {
	if svc.Tunnel == "" {
		return nil
	}

	// Extract IP from upstream URL
	serviceIP, err := a.extractIPFromUpstream(svc.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream %s: %w", svc.Upstream, err)
	}

	// Get the tunnel configuration
	tunnel := a.cfg.GetTunnel(svc.Tunnel)
	if tunnel == nil {
		return fmt.Errorf("tunnel %s not found", svc.Tunnel)
	}

	// Create route in CIDR format
	route := serviceIP + "/32"

	// Remove route from tunnel configuration
	newRoutes := make([]string, 0, len(tunnel.Routes))
	routeRemoved := false

	for _, existingRoute := range tunnel.Routes {
		if existingRoute != route {
			newRoutes = append(newRoutes, existingRoute)
		} else {
			routeRemoved = true
		}
	}

	if !routeRemoved {
		a.logger.Debug("Route not found in tunnel", "route", route, "tunnel", svc.Tunnel)
		return nil
	}

	tunnel.Routes = newRoutes

	// Update tunnel in config
	if err := a.cfg.UpdateTunnel(*tunnel); err != nil {
		return fmt.Errorf("failed to update tunnel config: %w", err)
	}

	// Update the running tunnel if it exists
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.tunnelManager.UpdateTunnel(ctx, *tunnel); err != nil {
		a.logger.Warn("Failed to update running tunnel, route removal will take effect on next start",
			"tunnel", svc.Tunnel, "route", route, "error", err)
	}

	a.logger.Info("Removed service route from tunnel",
		"service", svc.Name, "tunnel", svc.Tunnel, "route", route)

	return nil
}

// MARK: extractIPFromUpstream
func (a *APIServer) extractIPFromUpstream(upstream string) (string, error) {
	// Parse the upstream URL
	parsedURL, err := url.Parse(upstream)
	if err != nil {
		return "", fmt.Errorf("invalid upstream URL: %w", err)
	}

	// Extract hostname from URL
	hostname := parsedURL.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("no hostname found in upstream URL")
	}

	// If it's already an IP address, return it
	if net.ParseIP(hostname) != nil {
		return hostname, nil
	}

	// Resolve hostname to IP
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hostname %s: %w", hostname, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for hostname %s", hostname)
	}

	// Prefer IPv4 addresses
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			return ipv4.String(), nil
		}
	}

	// Fallback to first IP (might be IPv6)
	return ips[0].String(), nil
}

// MARK: validateServiceConfig

// Validates service configuration parameters
func (a *APIServer) validateServiceConfig(svc config.ServiceConfig) error {
	if svc.Name == "" || svc.Upstream == "" {
		return fmt.Errorf("name and upstream are required")
	}
	return nil
}

// MARK: publishServiceMDNS

// Publishes service via mDNS if enabled
func (a *APIServer) publishServiceMDNS(svc config.ServiceConfig) {
	if !svc.PublishMDNS || !a.cfg.Discovery.Enable || !a.cfg.Discovery.MDNS.Enabled || a.discoveryManager == nil {
		return
	}

	proxyPort := config.GetPortFromAddr(a.cfg.Server.ProxyAddr)
	if err := a.discoveryManager.PublishService(svc, proxyPort); err != nil {
		fmt.Printf("Failed to publish service via mDNS: %v\n", err)
	}
}

// MARK: handleGetService

// Returns status information for a specific service
func (a *APIServer) handleGetService(w http.ResponseWriter, r *http.Request, serviceName string) {
	status, err := a.proxyServer.GetServiceStatus(serviceName)
	if err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found")
		return
	}

	response := ServiceStatusResponse{
		Name:     status.Config.Name,
		Upstream: status.Config.Upstream,
		Status:   "running",
		Tunnel:   status.Config.Tunnel,
	}

	a.respondWithSuccess(w, "Service retrieved", response)
}

// MARK: handleTunnels

// Routes tunnel requests based on HTTP method
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

// MARK: handleListTunnels

// Returns list of all configured tunnels with their runtime status
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

// MARK: handleTunnelByName

// Routes individual tunnel requests based on HTTP method
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

// MARK: handleGetTunnel

// Returns status information for a specific tunnel
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

// Removes a tunnel from configuration and stops it
func (a *APIServer) handleDeleteTunnel(w http.ResponseWriter, r *http.Request, ctx context.Context, tunnelName string) {
	if err := a.cfg.RemoveTunnel(tunnelName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Tunnel not found in config")
		return
	}

	a.tunnelManager.DeleteTunnel(ctx, tunnelName)
	a.respondWithSuccess(w, fmt.Sprintf("Tunnel %s deleted successfully", tunnelName), nil)
}

// MARK: handleCreateTunnel

// Creates a new tunnel configuration and starts it
func (a *APIServer) handleCreateTunnel(w http.ResponseWriter, r *http.Request) {
	var req TunnelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if err := a.validateTunnelRequest(req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	tunnelCfg := a.buildTunnelConfig(req)

	if err := a.cfg.AddTunnel(tunnelCfg); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, "Failed to save tunnel: "+err.Error())
		return
	}

	ctx := r.Context()
	if err := a.tunnelManager.CreateTunnel(ctx, tunnelCfg); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, "Failed to create tunnel: "+err.Error())
		return
	}

	a.respondWithSuccess(w, fmt.Sprintf("Tunnel %s created successfully", req.Name), tunnelCfg)
}

// MARK: validateTunnelRequest

// Validates tunnel creation request parameters
func (a *APIServer) validateTunnelRequest(req TunnelCreateRequest) error {
	if req.Name == "" {
		return fmt.Errorf("tunnel name is required")
	}

	if req.PrivateKey == "" {
		return fmt.Errorf("private_key is required")
	}

	for i, peerReq := range req.Peers {
		if peerReq.PublicKey == "" {
			return fmt.Errorf("peer %d public_key is required", i)
		}
	}

	return nil
}

// MARK: buildTunnelConfig

// Converts API request to internal tunnel configuration
func (a *APIServer) buildTunnelConfig(req TunnelCreateRequest) config.TunnelConfig {
	if req.MTU == 0 {
		req.MTU = 1420
	}

	peers := make([]config.PeerConfig, len(req.Peers))
	for i, peerReq := range req.Peers {
		peers[i] = config.PeerConfig{
			Name:       peerReq.Name,
			PublicKey:  peerReq.PublicKey,
			AllowedIPs: peerReq.AllowedIPs,
			Endpoint:   peerReq.Endpoint,
			Preshared:  peerReq.PresharedKey,
			Persistent: peerReq.PersistentKeepalive > 0,
		}
	}

	return config.TunnelConfig{
		Name:       req.Name,
		ListenPort: req.ListenPort,
		PrivateKey: req.PrivateKey,
		MTU:        req.MTU,
		Addresses:  req.Addresses,
		Routes:     req.Routes,
		Peers:      peers,
	}
}

// MARK: handleStatus
func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ipList, err := utilities.GetSystemIPv4s()
	if err != nil {
		a.logger.Warn("Failed to get system IP", "error", err)
		ipList = []string{}
	}

	interfaces, err := utilities.GetInterfaceDetails()
	if err != nil {
		a.logger.Warn("Failed to get interface details", "error", err)
	}

	status := map[string]interface{}{
		"proxy":      a.proxyServer.IsReady(),
		"tunnels":    a.tunnelManager.IsReady(),
		"services":   len(a.proxyServer.ListServices()),
		"uptime":     time.Now().Format(time.RFC3339),
		"system_ip":  ipList,
		"interfaces": interfaces,
	}

	a.respondWithSuccess(w, "System status", status)
}

// MARK: handleLogs

// Returns application logs with pagination
func (a *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limit, offset := a.parsePaginationParams(r)
	level := r.URL.Query().Get("level")

	// Get logs from the logger
	allLogs := a.logger.GetLogs(level)

	// Apply pagination
	total := len(allLogs)
	start := offset
	if start >= total {
		allLogs = []internal.LogEntry{}
	} else {
		end := start + limit
		if end > total {
			end = total
		}
		allLogs = allLogs[start:end]
	}

	response := LogResponse{
		Logs:   convertLogEntries(allLogs),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}

	a.respondWithSuccess(w, "Logs retrieved", response)
}

// MARK: convertLogEntries

// Extracts logs and turns then into API ready records for logging
func convertLogEntries(internalLogs []internal.LogEntry) []LogEntry {
	logs := make([]LogEntry, len(internalLogs))
	for i, log := range internalLogs {
		logs[i] = LogEntry{
			Timestamp: utilities.ParseTimestamp(log.Timestamp),
			Level:     log.Level,
			Message:   log.Message,
			Context:   log.Context,
		}
	}
	return logs
}

// MARK: parsePaginationParams

// Extracts and validates pagination parameters from request
func (a *APIServer) parsePaginationParams(r *http.Request) (int, int) {
	limit := 100
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}

// MARK: respondWithSuccess

// Sends successful JSON response with data
func (a *APIServer) respondWithSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response := APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
	json.NewEncoder(w).Encode(response)
}

// MARK: respondWithError

// Sends error JSON response with appropriate status code
func (a *APIServer) respondWithError(w http.ResponseWriter, statusCode int, errorMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := APIResponse{
		Success: false,
		Error:   errorMessage,
	}
	json.NewEncoder(w).Encode(response)
}
