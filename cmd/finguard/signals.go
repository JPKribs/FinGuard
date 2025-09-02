package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: handleSignals
// Sets up signal handlers for graceful shutdown and config reload
func (app *Application) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()

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
			case <-app.context.Done():
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

	app.config = newCfg

	if err := app.addServices(); err != nil {
		app.logger.Error("Failed to add services during reload", "error", err)
	}

	app.publishServices()
	app.updateReadiness()

	app.logger.Info("Configuration reloaded successfully")
}
