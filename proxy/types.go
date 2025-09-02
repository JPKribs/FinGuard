package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

// MARK: ProxyService
type ProxyService struct {
	Config   config.ServiceConfig
	Upstream *url.URL
	Proxy    *httputil.ReverseProxy
	Health   *ServiceHealth
	mu       sync.RWMutex
}

// MARK: responseWriter
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// MARK: Server
type Server struct {
	logger   *internal.Logger
	services map[string]*ProxyService
	server   *http.Server
	running  bool
	mu       sync.RWMutex
}

// MARK: ServiceHealth
type ServiceHealth struct {
	Healthy     bool      `json:"healthy"`
	LastCheck   time.Time `json:"last_check"`
	LastError   string    `json:"last_error,omitempty"`
	Consecutive int       `json:"consecutive_failures"`
}
