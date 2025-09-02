package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: handleUpdateStatus
func (a *APIServer) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if a.updateManager == nil {
		a.respondWithError(w, http.StatusServiceUnavailable, "Update manager not available")
		return
	}

	status := a.updateManager.GetUpdateStatus()
	a.respondWithSuccess(w, "Update status retrieved", status)
}

// MARK: handleUpdateCheck
func (a *APIServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if a.updateManager == nil {
		a.respondWithError(w, http.StatusServiceUnavailable, "Update manager not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	info, err := a.updateManager.CheckForUpdates(ctx)
	if err != nil {
		a.respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to check for updates: %v", err))
		return
	}

	a.respondWithSuccess(w, "Update check completed", info)
}

// MARK: handleUpdateApply
func (a *APIServer) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if a.updateManager == nil {
		a.respondWithError(w, http.StatusServiceUnavailable, "Update manager not available")
		return
	}

	a.logger.Info("Manual update requested via API")

	a.respondWithSuccess(w, "Update initiated - application will restart if successful", nil)

	go func() {
		time.Sleep(1 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		if err := a.updateManager.PerformUpdate(ctx); err != nil {
			a.logger.Error("Manual update failed", "error", err)
		}
	}()
}

// MARK: handleUpdateConfig
func (a *APIServer) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleGetUpdateConfig(w, r)
	case http.MethodPost:
		a.handleSetUpdateConfig(w, r)
	default:
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// MARK: handleGetUpdateConfig
func (a *APIServer) handleGetUpdateConfig(w http.ResponseWriter, r *http.Request) {
	config := UpdateConfigRequest{
		Enabled:   a.cfg.Update.Enabled,
		Schedule:  a.cfg.Update.Schedule,
		AutoApply: a.cfg.Update.AutoApply,
		BackupDir: a.cfg.Update.BackupDir,
	}

	a.respondWithSuccess(w, "Update configuration retrieved", config)
}

// MARK: handleSetUpdateConfig
func (a *APIServer) handleSetUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req UpdateConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Schedule == "" {
		req.Schedule = "0 3 * * *"
	}
	if req.BackupDir == "" {
		req.BackupDir = "./backups"
	}

	updateConfig := config.UpdateConfig{
		Enabled:   req.Enabled,
		Schedule:  req.Schedule,
		AutoApply: req.AutoApply,
		BackupDir: req.BackupDir,
	}

	if err := a.cfg.UpdateUpdateConfig(updateConfig); err != nil {
		a.respondWithError(w, http.StatusBadRequest, "Invalid configuration: "+err.Error())
		return
	}

	if a.updateManager != nil {
		if err := a.updateManager.UpdateSchedule(req.Schedule); err != nil {
			a.logger.Warn("Failed to update scheduler", "error", err)
		}
	}

	a.respondWithSuccess(w, "Update configuration saved", updateConfig)
}
