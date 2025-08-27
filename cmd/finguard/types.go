package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/discovery"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/wireguard"
)

type Application struct {
	config           *config.Config
	logger           *internal.Logger
	healthCheck      *internal.HealthChecker
	tunnelManager    wireguard.TunnelManager
	proxyServer      *proxy.Server
	discoveryManager *discovery.Discovery
	server           *http.Server
	context          context.Context
	cancel           context.CancelFunc
	waitGroup        sync.WaitGroup
}
