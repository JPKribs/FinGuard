package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

type Server struct {
	logger   *internal.Logger
	services map[string]*ProxyService
	server   *http.Server
	running  bool
	mu       sync.RWMutex
}

type ServiceHealth struct {
	Healthy     bool      `json:"healthy"`
	LastCheck   time.Time `json:"last_check"`
	LastError   string    `json:"last_error,omitempty"`
	Consecutive int       `json:"consecutive_failures"`
}

type ProxyService struct {
	Config   config.ServiceConfig
	Upstream *url.URL
	Proxy    *httputil.ReverseProxy
	Health   *ServiceHealth
	mu       sync.RWMutex
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// MARK: NewServer

// Creates a new proxy server instance with logger
func NewServer(logger *internal.Logger) *Server {
	return &Server{
		logger:   logger,
		services: make(map[string]*ProxyService),
	}
}

// MARK: Start

// Starts the HTTP proxy server with routing and middleware
func (s *Server) Start(ctx context.Context, addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("proxy server already running")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	s.server = &http.Server{
		Addr:           addr,
		Handler:        s.withMinimalMiddleware(mux),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 20 << 20,
	}

	go func() {
		s.logger.Info("Starting proxy server", "addr", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Proxy server failed", "error", err)
		}
	}()

	// Start health checking in background
	go s.StartHealthChecking(ctx)

	s.running = true
	return nil
}

// MARK: Stop

// Gracefully shuts down the proxy server
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("Stopping proxy server")

	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return fmt.Errorf("shutting down proxy server: %w", err)
		}
	}

	s.running = false
	return nil
}

// MARK: AddService

// Configures a new service with reverse proxy and custom transport
func (s *Server) AddService(svc config.ServiceConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if service already exists
	if _, exists := s.services[svc.Name]; exists {
		s.logger.Warn("Service already exists, updating", "name", svc.Name)
		delete(s.services, svc.Name)
	}

	upstream, err := url.Parse(svc.Upstream)
	if err != nil {
		return fmt.Errorf("parsing upstream URL %s: %w", svc.Upstream, err)
	}

	// Create custom transport for this service with better timeouts
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(upstream)
			pr.Out.Host = upstream.Host
			s.setProxyHeaders(pr, svc)
		},
		ModifyResponse: func(resp *http.Response) error {
			s.setSecurityHeaders(resp)
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			s.handleProxyError(w, r, svc, err)
		},
	}

	service := &ProxyService{
		Config:   svc,
		Upstream: upstream,
		Proxy:    proxy,
		Health:   &ServiceHealth{Healthy: true, LastCheck: time.Now()},
	}

	s.services[svc.Name] = service
	s.logger.Info("Added service", "name", svc.Name, "upstream", svc.Upstream)
	return nil
}

// MARK: handleProxyError

// Enhanced error handler that categorizes and logs different proxy error types
func (s *Server) handleProxyError(w http.ResponseWriter, r *http.Request, svc config.ServiceConfig, err error) {
	var statusCode int
	var errorType string

	switch {
	case strings.Contains(err.Error(), "context canceled"):
		statusCode = http.StatusRequestTimeout
		errorType = "timeout"
		s.logger.Warn("Request timeout",
			"service", svc.Name,
			"upstream", svc.Upstream,
			"host", r.Host,
			"path", r.URL.Path,
			"method", r.Method,
			"remote", s.getClientIP(r))

	case strings.Contains(err.Error(), "connection refused"):
		statusCode = http.StatusBadGateway
		errorType = "connection_refused"
		s.logger.Error("Upstream connection refused",
			"service", svc.Name,
			"upstream", svc.Upstream,
			"host", r.Host,
			"error", err.Error())

	case strings.Contains(err.Error(), "no such host"):
		statusCode = http.StatusBadGateway
		errorType = "dns_failure"
		s.logger.Error("DNS resolution failed",
			"service", svc.Name,
			"upstream", svc.Upstream,
			"host", r.Host,
			"error", err.Error())

	case strings.Contains(err.Error(), "timeout"):
		statusCode = http.StatusGatewayTimeout
		errorType = "upstream_timeout"
		s.logger.Error("Upstream timeout",
			"service", svc.Name,
			"upstream", svc.Upstream,
			"host", r.Host,
			"timeout_duration", "15s",
			"error", err.Error())

	default:
		statusCode = http.StatusBadGateway
		errorType = "proxy_error"
		s.logger.Error("Proxy error",
			"service", svc.Name,
			"upstream", svc.Upstream,
			"host", r.Host,
			"error_type", errorType,
			"error", err.Error())
	}

	w.Header().Set("Content-Type", "text/plain")
	http.Error(w, fmt.Sprintf("Service temporarily unavailable (%s)", errorType), statusCode)
}

// MARK: StartHealthChecking

// Starts periodic health checks for all upstream services
func (s *Server) StartHealthChecking(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAllServices()
		}
	}
}

// MARK: checkAllServices

// Initiates health checks for all registered services concurrently
func (s *Server) checkAllServices() {
	s.mu.RLock()
	services := make([]*ProxyService, 0, len(s.services))
	for _, svc := range s.services {
		services = append(services, svc)
	}
	s.mu.RUnlock()

	for _, svc := range services {
		go s.checkServiceHealth(svc)
	}
}

// MARK: checkServiceHealth

// Performs health check on individual service and updates health status
func (s *Server) checkServiceHealth(service *ProxyService) {
	service.mu.Lock()
	defer service.mu.Unlock()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
		},
	}

	healthURL := service.Upstream.String()
	if !strings.HasSuffix(healthURL, "/") {
		healthURL += "/"
	}

	resp, err := client.Get(healthURL)
	now := time.Now()

	if service.Health == nil {
		service.Health = &ServiceHealth{}
	}

	service.Health.LastCheck = now

	if err != nil || (resp != nil && resp.StatusCode >= 500) {
		service.Health.Healthy = false
		service.Health.Consecutive++
		if err != nil {
			service.Health.LastError = err.Error()
		} else {
			service.Health.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
			resp.Body.Close()
		}

		if service.Health.Consecutive >= 3 {
			s.logger.Error("Service unhealthy",
				"name", service.Config.Name,
				"upstream", service.Config.Upstream,
				"consecutive_failures", service.Health.Consecutive,
				"error", service.Health.LastError)
		}
	} else {
		wasUnhealthy := !service.Health.Healthy
		service.Health.Healthy = true
		service.Health.Consecutive = 0
		service.Health.LastError = ""

		if resp != nil {
			resp.Body.Close()
		}

		if wasUnhealthy {
			s.logger.Info("Service recovered",
				"name", service.Config.Name,
				"upstream", service.Config.Upstream)
		}
	}
}

// MARK: RemoveService

// Removes a configured service from the proxy
func (s *Server) RemoveService(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.services[name]; !exists {
		return fmt.Errorf("service %s not found", name)
	}

	delete(s.services, name)
	s.logger.Info("Removed service", "name", name)
	return nil
}

// MARK: ListServices

// Returns all configured service configurations
func (s *Server) ListServices() []config.ServiceConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	services := make([]config.ServiceConfig, 0, len(s.services))
	for _, svc := range s.services {
		services = append(services, svc.Config)
	}
	return services
}

// MARK: GetServiceStatus

// Returns the proxy service instance for status checking
func (s *Server) GetServiceStatus(name string) (*ProxyService, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	service, exists := s.services[name]
	if !exists {
		return nil, fmt.Errorf("service %s not found", name)
	}

	return service, nil
}

// MARK: IsReady

// Returns true if the proxy server is running
func (s *Server) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// MARK: handleRequest

// Routes incoming requests to appropriate service based on hostname
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	service := s.findServiceByHost(r.Host)
	s.mu.RUnlock()

	if service == nil {
		s.logger.Debug("No service found", "host", r.Host)
		http.NotFound(w, r)
		return
	}

	service.Proxy.ServeHTTP(w, r)
}

// MARK: findServiceByHost

// Matches hostname to service using subdomain pattern matching
func (s *Server) findServiceByHost(host string) *ProxyService {
	host = s.stripPort(host)
	var defaultService *ProxyService

	for _, service := range s.services {
		if service.Config.Default {
			defaultService = service
		}

		if host == service.Config.Name+".local" || host == service.Config.Name {
			return service
		}
	}

	return defaultService
}

// MARK: setProxyHeaders

// Sets required headers for proxying including WebSocket support
func (s *Server) setProxyHeaders(pr *httputil.ProxyRequest, svc config.ServiceConfig) {
	pr.Out.Header.Set("X-Real-IP", s.getClientIP(pr.In))
	pr.Out.Header.Set("X-Forwarded-For", s.getClientIP(pr.In))
	pr.Out.Header.Set("X-Forwarded-Proto", s.getScheme(pr.In))
	pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)

	if svc.Websocket && s.isWebSocketUpgrade(pr.In) {
		s.setWebSocketHeaders(pr)
	}
}

// MARK: setWebSocketHeaders

// Copies WebSocket upgrade headers for proper connection handling
func (s *Server) setWebSocketHeaders(pr *httputil.ProxyRequest) {
	headers := []string{"Upgrade", "Connection", "Sec-WebSocket-Key", "Sec-WebSocket-Version", "Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"}

	for _, header := range headers {
		if value := pr.In.Header.Get(header); value != "" {
			pr.Out.Header.Set(header, value)
		}
	}
}

// MARK: setSecurityHeaders

// Adds security headers to non-WebSocket responses
func (s *Server) setSecurityHeaders(resp *http.Response) {
	if resp.StatusCode != 101 {
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		resp.Header.Set("X-Frame-Options", "SAMEORIGIN")
		resp.Header.Del("Server")
	}
}

// MARK: withMinimalMiddleware

// Applies logging and recovery middleware stack
func (s *Server) withMinimalMiddleware(handler http.Handler) http.Handler {
	return s.loggingMiddleware(s.recoveryMiddleware(handler))
}

// MARK: loggingMiddleware

// Logs HTTP requests with timing and status information
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isWebSocketUpgrade(r) {
			s.logger.Info("WebSocket", "host", r.Host, "remote", s.getClientIP(r))
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r)

		s.logger.Info("Request",
			"method", r.Method,
			"host", r.Host,
			"status", rw.statusCode,
			"duration", time.Since(start).String(),
			"remote", s.getClientIP(r))
	})
}

// MARK: recoveryMiddleware

// Recovers from panics and logs errors
func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Error("Panic", "error", err, "host", r.Host, "remote", s.getClientIP(r))
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// MARK: WriteHeader

// Captures HTTP status code for logging
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// MARK: Write

// Writes response body and sets default status code
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// MARK: stripPort

// Removes port from hostname if present
func (s *Server) stripPort(host string) string {
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		return host[:colonIndex]
	}
	return host
}

// MARK: getClientIP

// Extracts real client IP from headers or connection
func (s *Server) getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if commaIndex := strings.Index(xff, ","); commaIndex != -1 {
			return strings.TrimSpace(xff[:commaIndex])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	if colonIndex := strings.LastIndex(r.RemoteAddr, ":"); colonIndex != -1 {
		return r.RemoteAddr[:colonIndex]
	}
	return r.RemoteAddr
}

// MARK: getScheme

// Determines HTTP scheme from TLS status or headers
func (s *Server) getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

// MARK: isWebSocketUpgrade

// Checks if request is WebSocket upgrade
func (s *Server) isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}
