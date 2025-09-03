package v1

import (
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/utilities"
)

// MARK: handleSystemRestart
// Handle the system's restart on a OS level.
func (a *APIServer) handleSystemRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	a.logger.Info("System restart requested via API")
	a.respondWithSuccess(w, "Restart initiated", nil)

	go func() {
		time.Sleep(1 * time.Second)
		if !a.trySystemdRestart() {
			a.signalRestart()
		}
	}()
}

// MARK: handleSystemShutdown
// Handle the system's shutdown on a OS level.
func (a *APIServer) handleSystemShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	a.logger.Info("System shutdown requested via API")
	a.respondWithSuccess(w, "Shutdown initiated", nil)

	go func() {
		time.Sleep(1 * time.Second)
		if !a.trySystemdStop() {
			a.signalShutdown()
		}
	}()
}

// MARK: handleStatus
// Handle the system's statuses for each of the components.
func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ipv4List, err := utilities.GetSystemIPv4s()
	if err != nil {
		a.logger.Warn("Failed to get IPv4 addresses", "error", err)
		ipv4List = []string{}
	}

	ipv6List, err := utilities.GetSystemIPv6s()
	if err != nil {
		a.logger.Warn("Failed to get IPv6 addresses", "error", err)
		ipv6List = []string{}
	}

	interfaces, err := utilities.GetInterfaceDetails()
	if err != nil {
		a.logger.Warn("Failed to get interface details", "error", err)
	}

	var jellyfinStatus map[string]interface{}
	if a.jellyfinBroadcaster != nil {
		serviceCount := 0
		if a.jellyfinBroadcaster.HasJellyfinServices() {
			services := a.proxyServer.ListServices()
			for _, svc := range services {
				if svc.Jellyfin {
					serviceCount++
				}
			}
		}

		jellyfinStatus = map[string]interface{}{
			"running":  a.jellyfinBroadcaster.IsRunning(),
			"services": serviceCount,
		}
	} else {
		jellyfinStatus = map[string]interface{}{
			"running":  false,
			"services": 0,
		}
	}

	status := map[string]interface{}{
		"proxy":      a.proxyServer.IsReady(),
		"tunnels":    a.tunnelManager.IsReady(),
		"jellyfin":   jellyfinStatus,
		"services":   len(a.proxyServer.ListServices()),
		"uptime":     time.Now().Format(time.RFC3339),
		"ipv4":       ipv4List,
		"ipv6":       ipv6List,
		"interfaces": interfaces,
	}

	a.respondWithSuccess(w, "System status", status)
}

// MARK: trySystemdRestart
// Try to restart using Systemd (Debian).
func (a *APIServer) trySystemdRestart() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		a.logger.Debug("systemctl not found, using fallback method")
		return false
	}

	cmd := exec.Command("systemctl", "restart", "finguard")
	if err := cmd.Run(); err != nil {
		a.logger.Warn("systemctl restart failed", "error", err)
		return false
	}

	a.logger.Info("Successfully initiated restart via systemctl")
	return true
}

// MARK: trySystemdStop
// Try to stop using Systemd (Debian).
func (a *APIServer) trySystemdStop() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		a.logger.Debug("systemctl not found, using fallback method")
		return false
	}

	cmd := exec.Command("systemctl", "stop", "finguard")
	if err := cmd.Run(); err != nil {
		a.logger.Warn("systemctl stop failed", "error", err)
		return false
	}

	a.logger.Info("Successfully initiated stop via systemctl")
	return true
}

// MARK: signalRestart
// Signal that a restart is occurring.
func (a *APIServer) signalRestart() {
	internal.SetRestartFlag(true)
	a.signalShutdown()
}

// MARK: signalShutdown
// Signal that a shutdown is occurring.
func (a *APIServer) signalShutdown() {
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		a.logger.Error("Failed to find process", "error", err)
		return
	}

	if err := process.Signal(os.Interrupt); err != nil {
		a.logger.Error("Failed to send shutdown signal", "error", err)
	}
}
