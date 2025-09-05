package main

import (
	"context"
	"fmt"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/discovery"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/mdns"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/updater"
	"github.com/JPKribs/FinGuard/utilities"
	"github.com/JPKribs/FinGuard/version"
	"github.com/JPKribs/FinGuard/wireguard"
)

// MARK: newApplication
func newApplication(configPath string) (*Application, error) {
	config, error := config.Load(configPath)
	if error != nil {
		return nil, fmt.Errorf("loading config: %w", error)
	}

	// Load WireGuard configuration with defaults
	if err := config.LoadWireGuardWithDefaults(); err != nil {
		return nil, fmt.Errorf("loading WireGuard config: %w", err)
	}

	logger := internal.NewLogger(config.Log.Level)
	healthCheck := internal.NewHealthChecker()

	updateManager := updater.NewUpdateManager(config, logger, version.Version)
	jellyfinBroadcaster := discovery.NewJellyfinBroadcaster(logger)

	// Convert config.WireGuardMode to wireguard.TunnelMode using string comparison
	wireGuardMode := config.WireGuard.GetWireGuardMode()
	var tunnelMode wireguard.TunnelMode
	switch string(wireGuardMode) {
	case "wg-quick":
		tunnelMode = wireguard.ModeWgQuick
	case "kernel":
		tunnelMode = wireguard.ModeKernel
	case "userspace":
		tunnelMode = wireguard.ModeUserspace
	case "auto":
		tunnelMode = wireguard.ModeAuto
	default:
		tunnelMode = wireguard.ModeUserspace
	}

	tunnelManager, err := wireguard.NewManager(tunnelMode, config.WireGuard.Paths, logger)
	if err != nil {
		return nil, fmt.Errorf("creating tunnel manager: %w", err)
	}

	return &Application{
		config:              config,
		logger:              logger,
		healthCheck:         healthCheck,
		tunnelManager:       tunnelManager,
		proxyServer:         proxy.NewServer(logger),
		discoveryManager:    mdns.NewDiscovery(logger),
		jellyfinBroadcaster: jellyfinBroadcaster,
		updateManager:       updateManager,
	}, nil
}

// MARK: start
func (app *Application) start(ctx context.Context) error {
	app.logger.Info("Starting FinGuard", "version", version.Version)

	if err := app.startTunnelManager(ctx); err != nil {
		return fmt.Errorf("starting tunnel manager: %w", err)
	}

	if err := app.startDiscovery(ctx); err != nil {
		return fmt.Errorf("starting discovery: %w", err)
	}

	if err := app.startJellyfinBroadcaster(ctx); err != nil {
		return fmt.Errorf("starting jellyfin broadcaster: %w", err)
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
	app.setupJellyfinServices()
	app.updateReadiness()

	return app.startManagementServer()
}

// MARK: startJellyfinBroadcaster
func (app *Application) startJellyfinBroadcaster(ctx context.Context) error {
	ips, err := utilities.GetSystemIPv4s()
	if err != nil || len(ips) == 0 {
		app.logger.Error("Could not determine local IP for Jellyfin broadcaster", "error", err)
		return nil
	}
	localIP := ips[0]

	hostname := "finguard"

	if err := app.jellyfinBroadcaster.Start(localIP, hostname); err != nil {
		return fmt.Errorf("failed to start jellyfin broadcaster: %w", err)
	}

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
		<-ctx.Done()
		if err := app.jellyfinBroadcaster.Stop(); err != nil {
			app.logger.Error("Jellyfin broadcaster shutdown failed", "error", err)
		}
	}()

	return nil
}

// MARK: setupJellyfinServices
func (app *Application) setupJellyfinServices() {
	hasJellyfinServices := false

	for _, serviceCfg := range app.config.Services {
		if serviceCfg.Jellyfin {
			hasJellyfinServices = true
			if err := app.jellyfinBroadcaster.AddJellyfinService(serviceCfg.Name, serviceCfg.Upstream); err != nil {
				app.logger.Error("Failed to add Jellyfin service for broadcast",
					"name", serviceCfg.Name, "upstream", serviceCfg.Upstream, "error", err)
			} else {
				app.logger.Info("Added Jellyfin service for broadcast", "name", serviceCfg.Name)
			}
		}
	}

	if !hasJellyfinServices {
		app.logger.Info("No Jellyfin services found, skipping Jellyfin discovery setup")
	}
}
