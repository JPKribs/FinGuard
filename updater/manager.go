package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

	cmd := exec.CommandContext(ctx, "systemctl", "stop", sm.serviceName)
	return cmd.Run()
}

// MARK: StartService
func (sm *ServiceManager) StartService(ctx context.Context) error {
	if !sm.isSystemd {
		return fmt.Errorf("service start only supported on systemd systems")
	}

	sm.logger.Info("Starting service via systemd", "service", sm.serviceName)

	cmd := exec.CommandContext(ctx, "systemctl", "start", sm.serviceName)
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
	cmd := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", sm.serviceName)
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

	sm.logger.Info("Setting binary capabilities for TUN device access", "path", binaryPath)

	capabilities := []string{
		"cap_net_admin+ep",
		"cap_net_bind_service+ep",
	}

	for _, cap := range capabilities {
		cmd := exec.Command("sudo", "setcap", cap, binaryPath)
		if err := cmd.Run(); err != nil {
			sm.logger.Error("Failed to set capability", "capability", cap, "error", err)
			return fmt.Errorf("failed to set capability %s: %w", cap, err)
		}
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

	if os.Geteuid() != 0 {
		return fmt.Errorf("service requires root privileges for TUN device creation")
	}

	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return fmt.Errorf("TUN device not available: %w", err)
	}

	return nil
}
