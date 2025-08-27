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
