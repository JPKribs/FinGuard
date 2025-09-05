package updater

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/JPKribs/FinGuard/internal"
)

func NewServiceManager(serviceName string, logger *internal.Logger) *ServiceManager {
	return &ServiceManager{
		serviceName: serviceName,
		isSystemd:   detectSystemd(),
		logger:      logger,
	}
}

func detectSystemd() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func (sm *ServiceManager) RequestSystemdRestart() {
	sm.logger.Info("Signaling systemd restart")
	os.Exit(0)
}

func (sm *ServiceManager) KillZombieProcesses(processName string) error {
	sm.logger.Info("Killing zombie processes", "process", processName)
	cmd := exec.Command("pkill", "-f", processName)
	if err := cmd.Run(); err != nil {
		sm.logger.Debug("No processes found", "process", processName, "error", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}

// noop: set capabilities via systemd instead
func (sm *ServiceManager) SetCapabilities(binaryPath string) error {
	sm.logger.Info("Skipping setcap; systemd should manage capabilities")
	return nil
}

func (sm *ServiceManager) CleanupNetworkResources() error {
	sm.logger.Info("Cleaning up network resources")
	return nil
}

// ValidatePermissions does not require root
func (sm *ServiceManager) ValidatePermissions() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	// Only check if the binary is accessible
	if _, err := os.Stat(execPath); err != nil {
		return fmt.Errorf("binary not accessible: %w", err)
	}

	sm.logger.Info("Permissions validated - no sudo required")
	return nil
}
