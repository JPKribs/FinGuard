package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: handleServices
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
			Jellyfin:    svc.Jellyfin,
			Websocket:   svc.Websocket,
			Default:     svc.Default,
			PublishMDNS: svc.PublishMDNS,
		})
	}

	a.respondWithSuccess(w, "Services retrieved", statusList)
}

// MARK: handleAddService
func (a *APIServer) handleAddService(w http.ResponseWriter, r *http.Request) {
	var req ServiceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	serviceConfig := config.ServiceConfig{
		Name:        req.Name,
		Upstream:    req.Upstream,
		Tunnel:      req.Tunnel,
		Jellyfin:    req.Jellyfin,
		Websocket:   req.Websocket,
		Default:     req.Default,
		PublishMDNS: req.PublishMDNS,
	}

	if err := a.validateServiceConfig(serviceConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if serviceConfig.Tunnel != "" {
		if err := a.handleServiceTunnelRoute(&serviceConfig); err != nil {
			a.respondWithError(w, http.StatusInternalServerError, "Failed to manage tunnel route: "+err.Error())
			return
		}
	}

	if err := a.cfg.AddService(serviceConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.proxyServer.AddService(serviceConfig); err != nil {
		a.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.publishServiceMDNS(serviceConfig)

	response := ServiceStatusResponse{
		Name:        serviceConfig.Name,
		Upstream:    serviceConfig.Upstream,
		Status:      "running",
		Tunnel:      serviceConfig.Tunnel,
		Jellyfin:    serviceConfig.Jellyfin,
		Websocket:   serviceConfig.Websocket,
		Default:     serviceConfig.Default,
		PublishMDNS: serviceConfig.PublishMDNS,
	}

	a.respondWithSuccess(w, "Service added", response)
}

// MARK: handleDeleteService
func (a *APIServer) handleDeleteService(w http.ResponseWriter, r *http.Request, serviceName string) {
	services := a.proxyServer.ListServices()
	var serviceToDelete *config.ServiceConfig
	for i := range services {
		if strings.EqualFold(services[i].Name, serviceName) {
			serviceToDelete = &services[i]
			break
		}
	}

	if err := a.proxyServer.RemoveService(serviceName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found")
		return
	}

	if err := a.cfg.RemoveService(serviceName); err != nil {
		a.logger.Warn("Failed to remove service from config", "service", serviceName, "error", err)
	}

	if serviceToDelete != nil && serviceToDelete.Tunnel != "" {
		if err := a.removeServiceTunnelRoute(serviceToDelete); err != nil {
			a.logger.Error("Failed to remove tunnel route for deleted service",
				"service", serviceName, "tunnel", serviceToDelete.Tunnel, "error", err)
		}
	}

	if a.discoveryManager != nil {
		a.discoveryManager.UnpublishService(serviceName)
	}

	a.respondWithSuccess(w, "Service deleted", nil)
}

// MARK: handleGetService
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

// MARK: handleServiceTunnelRoute
func (a *APIServer) handleServiceTunnelRoute(serviceConfig *config.ServiceConfig) error {
	serviceIP, err := a.extractIPFromUpstream(serviceConfig.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream %s: %w", serviceConfig.Upstream, err)
	}

	tunnelConfig, err := a.findTunnelByName(serviceConfig.Tunnel)
	if err != nil {
		return fmt.Errorf("tunnel %s not found: %w", serviceConfig.Tunnel, err)
	}

	hostRoute := serviceIP + "/32"
	if !a.routeExists(tunnelConfig.Routes, hostRoute) {
		tunnelConfig.Routes = append(tunnelConfig.Routes, hostRoute)
		if err := a.cfg.UpdateTunnel(*tunnelConfig); err != nil {
			return fmt.Errorf("failed to update tunnel configuration: %w", err)
		}
		if err := a.restartTunnelForRouteChanges(serviceConfig.Tunnel); err != nil {
			a.logger.Error("Failed to restart tunnel after route addition",
				"tunnel", serviceConfig.Tunnel, "route", hostRoute, "error", err)
			return fmt.Errorf("failed to restart tunnel %s: %w", serviceConfig.Tunnel, err)
		}
		a.logger.Info("Added service route to tunnel",
			"service", serviceConfig.Name,
			"tunnel", serviceConfig.Tunnel,
			"route", hostRoute)
	}

	return nil
}

// MARK: removeServiceTunnelRoute
func (a *APIServer) removeServiceTunnelRoute(serviceConfig *config.ServiceConfig) error {
	serviceIP, err := a.extractIPFromUpstream(serviceConfig.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream: %w", err)
	}

	tunnelConfig, err := a.findTunnelByName(serviceConfig.Tunnel)
	if err != nil {
		return fmt.Errorf("tunnel not found: %w", err)
	}

	hostRoute := serviceIP + "/32"
	originalCount := len(tunnelConfig.Routes)
	newRoutes := make([]string, 0, originalCount)

	for _, r := range tunnelConfig.Routes {
		if r != hostRoute {
			newRoutes = append(newRoutes, r)
		}
	}

	if len(newRoutes) != originalCount {
		tunnelConfig.Routes = newRoutes
		if err := a.cfg.UpdateTunnel(*tunnelConfig); err != nil {
			return fmt.Errorf("failed to update tunnel configuration: %w", err)
		}
		if err := a.restartTunnelForRouteChanges(serviceConfig.Tunnel); err != nil {
			a.logger.Error("Failed to restart tunnel after route removal",
				"tunnel", serviceConfig.Tunnel, "route", hostRoute, "error", err)
			return fmt.Errorf("failed to restart tunnel: %w", err)
		}
		a.logger.Info("Removed service route from tunnel",
			"service", serviceConfig.Name,
			"tunnel", serviceConfig.Tunnel,
			"route", hostRoute)
	}

	return nil
}

// MARK: extractIPFromUpstream
func (a *APIServer) extractIPFromUpstream(upstream string) (string, error) {
	parsedURL, err := url.Parse(upstream)
	if err != nil {
		return "", fmt.Errorf("invalid upstream URL: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", fmt.Errorf("no hostname found in upstream URL")
	}

	if net.ParseIP(host) != nil {
		return host, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hostname %s: %w", host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for hostname %s", host)
	}

	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return ips[0].String(), nil
}

// MARK: findTunnelByName
func (a *APIServer) findTunnelByName(tunnelName string) (*config.TunnelConfig, error) {
	for i := range a.cfg.WireGuard.Tunnels {
		if strings.EqualFold(a.cfg.WireGuard.Tunnels[i].Name, tunnelName) {
			return &a.cfg.WireGuard.Tunnels[i], nil
		}
	}
	return nil, fmt.Errorf("tunnel %s not found", tunnelName)
}

// MARK: routeExists
func (a *APIServer) routeExists(routes []string, target string) bool {
	for _, r := range routes {
		if r == target {
			return true
		}
	}
	return false
}

// MARK: restartTunnelForRouteChanges
func (a *APIServer) restartTunnelForRouteChanges(tunnelName string) error {
	ctx := context.Background()

	if err := a.tunnelManager.DeleteTunnel(ctx, tunnelName); err != nil {
		a.logger.Warn("Failed to stop tunnel for restart", "tunnel", tunnelName, "error", err)
	}

	tunnelConfig, err := a.findTunnelByName(tunnelName)
	if err != nil {
		return err
	}

	if err := a.tunnelManager.CreateTunnel(ctx, *tunnelConfig); err != nil {
		return fmt.Errorf("failed to start tunnel %s: %w", tunnelName, err)
	}

	a.logger.Info("Tunnel restarted successfully", "tunnel", tunnelName)
	return nil
}

// MARK: validateServiceConfig
func (a *APIServer) validateServiceConfig(svc config.ServiceConfig) error {
	if svc.Name == "" || svc.Upstream == "" {
		return fmt.Errorf("name and upstream are required")
	}
	return nil
}

// MARK: publishServiceMDNS
func (a *APIServer) publishServiceMDNS(svc config.ServiceConfig) {
	if !svc.PublishMDNS || !a.cfg.Discovery.Enable || !a.cfg.Discovery.MDNS.Enabled || a.discoveryManager == nil {
		return
	}

	proxyPort := config.GetPortFromAddr(a.cfg.Server.ProxyAddr)
	if err := a.discoveryManager.PublishService(svc, proxyPort); err != nil {
		fmt.Printf("Failed to publish service via mDNS: %v\n", err)
	}
}
