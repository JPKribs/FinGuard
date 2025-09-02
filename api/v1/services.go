package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
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
		Websocket:   req.Websocket,
		Default:     req.Default,
		PublishMDNS: req.PublishMDNS,
	}

	if err := a.validateServiceConfig(serviceConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, err.Error())
		return
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
		Websocket:   serviceConfig.Websocket,
		Default:     serviceConfig.Default,
		PublishMDNS: serviceConfig.PublishMDNS,
	}

	a.respondWithSuccess(w, "Service added", response)
}

// MARK: handleDeleteService
func (a *APIServer) handleDeleteService(w http.ResponseWriter, r *http.Request, serviceName string) {
	if err := a.proxyServer.RemoveService(serviceName); err != nil {
		a.respondWithError(w, http.StatusNotFound, "Service not found")
		return
	}

	if err := a.cfg.RemoveService(serviceName); err != nil {
		a.logger.Warn("Failed to remove service from config", "service", serviceName, "error", err)
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
