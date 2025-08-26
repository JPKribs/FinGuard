package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	v1 "github.com/JPKribs/FinGuard/api/v1"
	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/discovery"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/wireguard"
)

const (
	Version         = "2.0.0"
	ShutdownTimeout = 30 * time.Second
	RetryDelay      = 5 * time.Second
	MaxRetries      = 3
)

type Application struct {
	cfg              *config.Config
	logger           *internal.Logger
	health           *internal.HealthChecker
	tunnelManager    wireguard.TunnelManager
	proxyServer      *proxy.Server
	discoveryManager *discovery.Discovery
	server           *http.Server
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// MARK: main

// Application entry point with command line parsing and error handling
func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to configuration file")
		version    = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *version {
		fmt.Printf("FinGuard-Go v%s\n", Version)
		os.Exit(0)
	}

	app, err := newApplication(*configPath)
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	app.ctx = ctx
	app.cancel = cancel
	defer cancel()

	if err := app.start(ctx); err != nil {
		log.Fatalf("Failed to start application: %v", err)
	}

	app.handleSignals()
	app.wg.Wait()
}

// MARK: newApplication

// Creates and configures a new application instance
func newApplication(configPath string) (*Application, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	logger := internal.NewLogger(cfg.Log.Level)
	health := internal.NewHealthChecker()

	return &Application{
		cfg:              cfg,
		logger:           logger,
		health:           health,
		tunnelManager:    wireguard.NewManager(logger),
		proxyServer:      proxy.NewServer(logger),
		discoveryManager: discovery.NewDiscovery(logger),
	}, nil
}

// MARK: start

// Initializes and starts all application components
func (app *Application) start(ctx context.Context) error {
	app.logger.Info("Starting FinGuard", "version", Version)

	if err := app.startTunnelManager(ctx); err != nil {
		return fmt.Errorf("starting tunnel manager: %w", err)
	}

	if err := app.startDiscovery(ctx); err != nil {
		return fmt.Errorf("starting discovery: %w", err)
	}

	if err := app.createTunnels(ctx); err != nil {
		app.logger.Error("Failed to create some tunnels", "error", err)
	}

	if err := app.startProxy(ctx); err != nil {
		return fmt.Errorf("starting proxy: %w", err)
	}

	if err := app.addServices(); err != nil {
		app.logger.Error("Failed to add some services", "error", err)
	}

	app.publishServices()
	app.updateReadiness()

	return app.startManagementServer()
}

// MARK: startTunnelManager

// Initializes the WireGuard tunnel manager
func (app *Application) startTunnelManager(ctx context.Context) error {
	if err := app.tunnelManager.Start(ctx); err != nil {
		return err
	}

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := app.tunnelManager.Stop(shutdownCtx); err != nil {
			app.logger.Error("Tunnel manager shutdown failed", "error", err)
		}
	}()

	return nil
}

// MARK: startDiscovery

// Starts mDNS service discovery if enabled
func (app *Application) startDiscovery(ctx context.Context) error {
	if !app.cfg.Discovery.Enable || !app.cfg.Discovery.MDNS.Enabled {
		return nil
	}

	if err := app.discoveryManager.Start(ctx); err != nil {
		return err
	}

	app.logger.Info("Started mDNS publisher")

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := app.discoveryManager.Stop(shutdownCtx); err != nil {
			app.logger.Error("Discovery manager shutdown failed", "error", err)
		}
	}()

	return nil
}

// MARK: createTunnels

// Creates all configured WireGuard tunnels with retry logic
func (app *Application) createTunnels(ctx context.Context) error {
	var errs []error

	for _, tunnelCfg := range app.cfg.WireGuard.Tunnels {
		if err := app.createTunnelWithRetry(ctx, tunnelCfg); err != nil {
			errs = append(errs, fmt.Errorf("tunnel %s: %w", tunnelCfg.Name, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// MARK: createTunnelWithRetry

// Creates a single tunnel with exponential backoff retry
func (app *Application) createTunnelWithRetry(ctx context.Context, tunnelCfg config.TunnelConfig) error {
	var lastErr error

	for attempt := 1; attempt <= MaxRetries; attempt++ {
		if err := app.tunnelManager.CreateTunnel(ctx, tunnelCfg); err != nil {
			lastErr = err
			app.logger.Error("Failed to create tunnel",
				"name", tunnelCfg.Name,
				"attempt", attempt,
				"max_attempts", MaxRetries,
				"error", err)

			if attempt < MaxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(RetryDelay * time.Duration(attempt)):
				}
			}
			continue
		}

		app.logger.Info("Created tunnel", "name", tunnelCfg.Name)
		return nil
	}

	return lastErr
}

// MARK: startProxy

// Initializes and starts the HTTP proxy server
func (app *Application) startProxy(ctx context.Context) error {
	if err := app.proxyServer.Start(ctx, app.cfg.Server.ProxyAddr); err != nil {
		return err
	}

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := app.proxyServer.Stop(shutdownCtx); err != nil {
			app.logger.Error("Proxy server shutdown failed", "error", err)
		}
	}()

	return nil
}

// MARK: addServices

// Adds all configured services to the proxy with error collection
func (app *Application) addServices() error {
	var errs []error
	addedServices := make(map[string]bool)

	for _, serviceCfg := range app.cfg.Services {
		// Skip if already added (prevents duplicates)
		if addedServices[serviceCfg.Name] {
			app.logger.Warn("Skipping duplicate service", "name", serviceCfg.Name)
			continue
		}

		if err := app.proxyServer.AddService(serviceCfg); err != nil {
			errs = append(errs, fmt.Errorf("service %s: %w", serviceCfg.Name, err))
			app.logger.Error("Failed to add service", "name", serviceCfg.Name, "error", err)
		} else {
			addedServices[serviceCfg.Name] = true
			app.logger.Info("Added service", "name", serviceCfg.Name, "upstream", serviceCfg.Upstream)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// MARK: publishServices

// Publishes configured services via mDNS
func (app *Application) publishServices() {
	if !app.cfg.Discovery.Enable || !app.cfg.Discovery.MDNS.Enabled {
		return
	}

	proxyPort := config.GetPortFromAddr(app.cfg.Server.ProxyAddr)

	for _, serviceCfg := range app.cfg.Services {
		if serviceCfg.PublishMDNS {
			if err := app.discoveryManager.PublishService(serviceCfg, proxyPort); err != nil {
				app.logger.Error("Failed to publish service via mDNS",
					"name", serviceCfg.Name, "error", err)
			} else {
				app.logger.Info("Published service via mDNS", "name", serviceCfg.Name)
			}
		}
	}
}

// MARK: updateReadiness

// Updates application readiness status based on component states
func (app *Application) updateReadiness() {
	isReady := app.tunnelManager.IsReady() && app.proxyServer.IsReady()

	if app.cfg.Discovery.Enable && app.cfg.Discovery.MDNS.Enabled {
		isReady = isReady && app.discoveryManager.IsReady()
	}

	app.health.SetReady(isReady)
}

// MARK: startManagementServer

// Starts the HTTP management API server
func (app *Application) startManagementServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.health.LivenessHandler)
	mux.HandleFunc("/readyz", app.health.ReadinessHandler)

	// Pass the logger to APIServer
	apiServer := v1.NewAPIServer(app.cfg, app.proxyServer, app.tunnelManager, app.discoveryManager, app.logger)
	apiServer.RegisterRoutes(mux)

	app.server = &http.Server{
		Addr:         app.cfg.Server.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		app.logger.Info("Starting management server", "addr", app.cfg.Server.HTTPAddr)

		if err := app.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("Management server failed", "error", err)
		}
	}()

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()
		<-app.ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		if err := app.server.Shutdown(shutdownCtx); err != nil {
			app.logger.Error("Server shutdown failed", "error", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	return nil
}

// MARK: handleSignals

// Processes OS signals for graceful shutdown and configuration reload
func (app *Application) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	app.wg.Add(1)
	go func() {
		defer app.wg.Done()

		for {
			select {
			case sig := <-sigChan:
				switch sig {
				case syscall.SIGHUP:
					app.handleReload()
				case syscall.SIGINT, syscall.SIGTERM:
					app.logger.Info("Received shutdown signal", "signal", sig)
					app.cancel()
					return
				}
			case <-app.ctx.Done():
				return
			}
		}
	}()
}

// MARK: handleReload

// Reloads configuration and updates running services
func (app *Application) handleReload() {
	app.logger.Info("Received SIGHUP, reloading configuration")

	newCfg, err := config.Load("config.yaml")
	if err != nil {
		app.logger.Error("Failed to reload config", "error", err)
		return
	}

	app.cfg = newCfg

	if err := app.addServices(); err != nil {
		app.logger.Error("Failed to add services during reload", "error", err)
	}

	app.publishServices()
	app.updateReadiness()

	app.logger.Info("Configuration reloaded successfully")
}
