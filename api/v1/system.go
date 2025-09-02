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
func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ipList, err := utilities.GetSystemIPv4s()
	if err != nil {
		a.logger.Warn("Failed to get system IP", "error", err)
		ipList = []string{}
	}

	interfaces, err := utilities.GetInterfaceDetails()
	if err != nil {
		a.logger.Warn("Failed to get interface details", "error", err)
	}

	status := map[string]interface{}{
		"proxy":      a.proxyServer.IsReady(),
		"tunnels":    a.tunnelManager.IsReady(),
		"services":   len(a.proxyServer.ListServices()),
		"uptime":     time.Now().Format(time.RFC3339),
		"system_ip":  ipList,
		"interfaces": interfaces,
	}

	a.respondWithSuccess(w, "System status", status)
}

// MARK: trySystemdRestart
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
func (a *APIServer) signalRestart() {
	internal.SetRestartFlag(true)
	a.signalShutdown()
}

// MARK: signalShutdown
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
