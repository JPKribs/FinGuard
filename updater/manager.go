package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/JPKribs/FinGuard/internal"
)

// MARK: NewServiceManager
func NewServiceManager(serviceName string, logger *internal.Logger) *ServiceManager {
	return &ServiceManager{
		serviceName: serviceName,
		isSystemd:   detectSystemd(),
		logger:      logger,
	}
}

// MARK: detectSystemd
func detectSystemd() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// MARK: RestartService
func (sm *ServiceManager) RestartService(ctx context.Context) error {
	if !sm.isSystemd {
		return fmt.Errorf("service restart only supported on systemd systems")
	}

	sm.logger.Info("Restarting service via systemd", "service", sm.serviceName)

	cmd := exec.CommandContext(ctx, "systemctl", "restart", sm.serviceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to restart service: %w", err)
	}

	return sm.waitForService(ctx, 30*time.Second)
}

// MARK: StopService
func (sm *ServiceManager) StopService(ctx context.Context) error {
	if !sm.isSystemd {
		return fmt.Errorf("service stop only supported on systemd systems")
	}

	sm.logger.Info("Stopping service via systemd", "service", sm.serviceName)

	cmd := exec.CommandContext(ctx, "sudo", "systemctl", "stop", sm.serviceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Treat "already stopped" (exit code 1) as non-critical
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			sm.logger.Warn("Service may already be stopped", "output", string(output))
			return nil
		}
		// Other errors are real failures
		return fmt.Errorf("failed to stop service: %w, output: %s", err, string(output))
	}

	// Wait until the service is actually inactive (up to 10s)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for service to stop")
		case <-ticker.C:
			if !sm.isServiceActive(ctx) {
				return nil
			}
		}
	}
}

// MARK: StartService
func (sm *ServiceManager) StartService(ctx context.Context) error {
	if !sm.isSystemd {
		return fmt.Errorf("service start only supported on systemd systems")
	}

	sm.logger.Info("Starting service via systemd", "service", sm.serviceName)

	cmd := exec.CommandContext(ctx, "sudo", "systemctl", "start", sm.serviceName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return sm.waitForService(ctx, 30*time.Second)
}

// MARK: waitForService
func (sm *ServiceManager) waitForService(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for service to be ready")
		case <-ticker.C:
			if sm.isServiceActive(ctx) {
				return nil
			}
		}
	}
}

// MARK: isServiceActive
func (sm *ServiceManager) isServiceActive(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "sudo", "systemctl", "is-active", "--quiet", sm.serviceName)
	return cmd.Run() == nil
}

// MARK: KillZombieProcesses
func (sm *ServiceManager) KillZombieProcesses(processName string) error {
	sm.logger.Info("Cleaning up zombie processes", "process", processName)

	cmd := exec.Command("pkill", "-f", processName)
	if err := cmd.Run(); err != nil {
		sm.logger.Warn("Failed to kill processes (this is normal if none exist)", "process", processName, "error", err)
	}

	time.Sleep(2 * time.Second)

	cmd = exec.Command("pkill", "-9", "-f", processName)
	if err := cmd.Run(); err != nil {
		sm.logger.Warn("Failed to force kill processes (this is normal if none exist)", "process", processName, "error", err)
	}

	return nil
}

// MARK: SetCapabilities
func (sm *ServiceManager) SetCapabilities(binaryPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	sm.logger.Info("Setting binary capabilities", "path", binaryPath)

	cmd := exec.Command("sudo", "setcap", "cap_net_admin,cap_net_raw,cap_net_bind_service+ep", binaryPath)
	if err := cmd.Run(); err != nil {
		sm.logger.Error("Failed to set capabilities", "error", err)
		return fmt.Errorf("failed to set capabilities: %w", err)
	}

	sm.logger.Info("Binary capabilities set successfully")
	return nil
}

// MARK: CleanupNetworkResources
func (sm *ServiceManager) CleanupNetworkResources() error {
	sm.logger.Info("Cleaning up network resources")

	cmd := exec.Command("pkill", "-f", "jellyfin")
	if err := cmd.Run(); err != nil {
		sm.logger.Debug("No jellyfin processes to kill", "error", err)
	}

	return nil
}

// MARK: ValidatePermissions
func (sm *ServiceManager) ValidatePermissions() error {
	if runtime.GOOS == "windows" {
		return nil
	}

	// Check if we can write to the binary location
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	// Check write permission to binary directory
	binDir := filepath.Dir(execPath)
	testFile := filepath.Join(binDir, ".update-test")
	if err := os.WriteFile(testFile, []byte("test"), 0755); err != nil {
		return fmt.Errorf("cannot write to binary directory %s: %w", binDir, err)
	}
	os.Remove(testFile)

	// Check if we have sudo access for setcap (just check sudo existence)
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("sudo not available for setting capabilities: %w", err)
	}

	return nil
}
