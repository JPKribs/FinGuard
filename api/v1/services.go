package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

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

	// Check for existing service BEFORE doing any operations
	if err := a.checkServiceExists(serviceConfig.Name); err != nil {
		a.respondWithError(w, http.StatusConflict, err.Error())
		return
	}

	// Add to config first - this validates and prevents duplicates
	if err := a.cfg.AddService(serviceConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Add tunnel route after config is saved
	var tunnelToUpdate *config.TunnelConfig
	if serviceConfig.Tunnel != "" {
		if err := a.addServiceRouteToTunnel(serviceConfig); err != nil {
			// Rollback config change
			a.cfg.RemoveService(serviceConfig.Name)
			a.respondWithError(w, http.StatusInternalServerError, "Failed to add route to tunnel: "+err.Error())
			return
		}
		tunnelToUpdate = a.cfg.GetTunnel(serviceConfig.Tunnel)
	}

	// Add to proxy server last
	if err := a.proxyServer.AddService(serviceConfig); err != nil {
		// Rollback both config and tunnel route
		a.cfg.RemoveService(serviceConfig.Name)
		if serviceConfig.Tunnel != "" {
			a.removeServiceRouteFromTunnel(serviceConfig)
		}
		a.respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	a.publishServiceMDNS(serviceConfig)

	if tunnelToUpdate != nil {
		a.logger.Info("Updating tunnel with new service route",
			"service", serviceConfig.Name, "tunnel", serviceConfig.Tunnel)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := a.updateRunningTunnel(ctx, *tunnelToUpdate); err != nil {
			a.logger.Warn("Failed to update tunnel configuration",
				"service", serviceConfig.Name, "tunnel", serviceConfig.Tunnel, "error", err)
		} else {
			a.logger.Info("Successfully updated tunnel with new route",
				"service", serviceConfig.Name, "tunnel", serviceConfig.Tunnel)
		}
	}

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

	successMessage := fmt.Sprintf("Service %s added successfully", serviceConfig.Name)
	if serviceConfig.Tunnel != "" {
		successMessage += fmt.Sprintf(" with route to tunnel %s", serviceConfig.Tunnel)
	}

	a.respondWithSuccess(w, successMessage, response)
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

	if serviceToDelete == nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found")
		return
	}

	if serviceToDelete.Tunnel != "" {
		if err := a.removeServiceRouteFromTunnel(*serviceToDelete); err != nil {
			a.logger.Error("Failed to remove route from tunnel", "error", err)
		}

		tunnelConfig := a.cfg.GetTunnel(serviceToDelete.Tunnel)
		if tunnelConfig != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := a.updateRunningTunnel(ctx, *tunnelConfig); err != nil {
				a.logger.Warn("Failed to update tunnel after service deletion",
					"service", serviceName, "tunnel", serviceToDelete.Tunnel, "error", err)
			}
		}
	}

	if err := a.proxyServer.RemoveService(serviceName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found in proxy")
		return
	}

	if err := a.cfg.RemoveService(serviceName); err != nil {
		a.logger.Warn("Failed to remove service from config", "service", serviceName, "error", err)
	}

	if a.discoveryManager != nil {
		a.discoveryManager.UnpublishService(serviceName)
	}

	a.respondWithSuccess(w, fmt.Sprintf("Service %s deleted successfully", serviceName), nil)
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

// MARK: addServiceRouteToTunnel
func (a *APIServer) addServiceRouteToTunnel(serviceConfig config.ServiceConfig) error {
	serviceIP, err := a.extractIPFromUpstream(serviceConfig.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream %s: %w", serviceConfig.Upstream, err)
	}

	tunnel := a.cfg.GetTunnel(serviceConfig.Tunnel)
	if tunnel == nil {
		return fmt.Errorf("tunnel %s not found", serviceConfig.Tunnel)
	}

	route := serviceIP + "/32"

	for _, existingRoute := range tunnel.Routes {
		if existingRoute == route {
			a.logger.Info("Route already exists for service", "service", serviceConfig.Name, "route", route)
			return nil
		}
	}

	tunnel.Routes = append(tunnel.Routes, route)

	if err := a.cfg.UpdateTunnel(*tunnel); err != nil {
		return fmt.Errorf("failed to update tunnel config: %w", err)
	}

	a.logger.Info("Added service route to tunnel",
		"service", serviceConfig.Name, "tunnel", serviceConfig.Tunnel, "route", route)

	return nil
}

// MARK: removeServiceRouteFromTunnel
func (a *APIServer) removeServiceRouteFromTunnel(serviceConfig config.ServiceConfig) error {
	if serviceConfig.Tunnel == "" {
		return nil
	}

	serviceIP, err := a.extractIPFromUpstream(serviceConfig.Upstream)
	if err != nil {
		return fmt.Errorf("failed to extract IP from upstream %s: %w", serviceConfig.Upstream, err)
	}

	tunnel := a.cfg.GetTunnel(serviceConfig.Tunnel)
	if tunnel == nil {
		return fmt.Errorf("tunnel %s not found", serviceConfig.Tunnel)
	}

	route := serviceIP + "/32"

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
		a.logger.Debug("Route not found in tunnel", "route", route, "tunnel", serviceConfig.Tunnel)
		return nil
	}

	tunnel.Routes = newRoutes

	if err := a.cfg.UpdateTunnel(*tunnel); err != nil {
		return fmt.Errorf("failed to update tunnel config: %w", err)
	}

	a.logger.Info("Removed service route from tunnel",
		"service", serviceConfig.Name, "tunnel", serviceConfig.Tunnel, "route", route)

	return nil
}

// MARK: updateRunningTunnel
func (a *APIServer) updateRunningTunnel(ctx context.Context, tunnelConfig config.TunnelConfig) error {
	status, err := a.tunnelManager.Status(ctx, tunnelConfig.Name)
	if err != nil {
		a.logger.Debug("Tunnel not running or status unavailable", "tunnel", tunnelConfig.Name, "error", err)
		return nil
	}

	if status.State != "running" {
		a.logger.Debug("Tunnel not in running state, skipping update", "tunnel", tunnelConfig.Name, "state", status.State)
		return nil
	}

	if err := a.tunnelManager.UpdateTunnel(ctx, tunnelConfig); err != nil {
		return fmt.Errorf("updating tunnel configuration: %w", err)
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

// MARK: checkServiceExists
func (a *APIServer) checkServiceExists(serviceName string) error {
	// Check config first (case-insensitive)
	for _, existing := range a.cfg.Services {
		if strings.EqualFold(existing.Name, serviceName) {
			return fmt.Errorf("service %s already exists in configuration", serviceName)
		}
	}

	// Check proxy server services (in case of config/runtime mismatch)
	services := a.proxyServer.ListServices()
	for _, svc := range services {
		if strings.EqualFold(svc.Name, serviceName) {
			a.logger.Warn("Service exists in proxy but not in config - this indicates a state mismatch",
				"service", serviceName, "proxy_upstream", svc.Upstream)
			return fmt.Errorf("service %s already exists in proxy server (state mismatch - restart application to sync)", serviceName)
		}
	}

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
