package main

import (
	"context"
	"fmt"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/mdns"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/updater"
	"github.com/JPKribs/FinGuard/version"
	"github.com/JPKribs/FinGuard/wireguard"
)

// MARK: newApplication
// Creates and configures a new application instance
func newApplication(configPath string) (*Application, error) {
	config, error := config.Load(configPath)
	if error != nil {
		return nil, fmt.Errorf("loading config: %w", error)
	}

	logger := internal.NewLogger(config.Log.Level)
	healthCheck := internal.NewHealthChecker()

	updateManager := updater.NewUpdateManager(config, logger, version.Version)

	return &Application{
		config:           config,
		logger:           logger,
		healthCheck:      healthCheck,
		tunnelManager:    wireguard.NewManager(logger),
		proxyServer:      proxy.NewServer(logger),
		discoveryManager: mdns.NewDiscovery(logger),
		updateManager:    updateManager,
	}, nil
}

// MARK: start
// Initializes and starts all application components
func (app *Application) start(ctx context.Context) error {
	app.logger.Info("Starting FinGuard", "version", version.Version)

	if err := app.startTunnelManager(ctx); err != nil {
		return fmt.Errorf("starting tunnel manager: %w", err)
	}

	if err := app.startDiscovery(ctx); err != nil {
		return fmt.Errorf("starting discovery: %w", err)
	}

	if err := app.startUpdateManager(ctx); err != nil {
		app.logger.Warn("Failed to start update manager", "error", err)
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
