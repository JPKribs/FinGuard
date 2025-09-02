package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	v1 "github.com/JPKribs/FinGuard/api/v1"
)

// MARK: startManagementServer
// Starts the HTTP management API server
func (app *Application) startManagementServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", app.healthCheck.LivenessHandler)
	mux.HandleFunc("/readyz", app.healthCheck.ReadinessHandler)

	apiServer := v1.NewAPIServer(app.config, app.proxyServer, app.tunnelManager, app.discoveryManager, app.logger, app.updateManager)
	apiServer.RegisterRoutes(mux)

	app.server = &http.Server{
		Addr:         app.config.Server.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
		app.logger.Info("Starting management server", "addr", app.config.Server.HTTPAddr)

		if err := app.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			app.logger.Error("Management server failed", "error", err)
		}
	}()

	app.waitGroup.Add(1)
	go func() {
		defer app.waitGroup.Done()
		<-app.context.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()

		if err := app.server.Shutdown(shutdownCtx); err != nil {
			app.logger.Error("Server shutdown failed", "error", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	return nil
}
