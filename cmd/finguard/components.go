package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: startUpdateManager
// Starts the auto-update manager component
func (app *Application) startUpdateManager(ctx context.Context) error {
	if app.updateManager == nil {
		return nil
	}

	if err := app.updateManager.Start(); err != nil {
		return err
	}

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
		<-ctx.Done()

		_, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		if err := app.updateManager.Stop(); err != nil {
			app.logger.Error("Update manager shutdown failed", "error", err)
		}
	}()

	return nil
}

// MARK: startTunnelManager
// Initializes the WireGuard tunnel manager
func (app *Application) startTunnelManager(ctx context.Context) error {
	if err := app.tunnelManager.Start(ctx); err != nil {
		return err
	}

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
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
	if !app.config.Discovery.Enable || !app.config.Discovery.MDNS.Enabled {
		return nil
	}

	if err := app.discoveryManager.Start(ctx); err != nil {
		return err
	}

	app.logger.Info("Started mDNS publisher")

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
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

	for _, tunnelCfg := range app.config.WireGuard.Tunnels {
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
	if err := app.proxyServer.Start(ctx, app.config.Server.ProxyAddr); err != nil {
		return err
	}

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		if err := app.proxyServer.Stop(shutdownCtx); err != nil {
			app.logger.Error("Proxy server shutdown failed", "error", err)
		}
	}()

	return nil
}
