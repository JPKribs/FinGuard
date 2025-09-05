package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
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

// MARK: RequestSystemdRestart
func (sm *ServiceManager) RequestSystemdRestart() {
	sm.logger.Info("Signaling systemd restart")
	os.Exit(0)
}

// MARK: KillZombieProcesses
func (sm *ServiceManager) KillZombieProcesses(processName string) error {
	sm.logger.Info("Killing zombie processes", "process", processName)

	cmd := exec.Command("pkill", "-f", processName)
	if err := cmd.Run(); err != nil {
		sm.logger.Debug("No processes found", "process", processName, "error", err)
	}

	time.Sleep(1 * time.Second)
	return nil
}

// MARK: SetCapabilities
func (sm *ServiceManager) SetCapabilities(binaryPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	sm.logger.Info("Setting binary capabilities", "path", binaryPath)

	cmd := exec.Command("setcap", "cap_net_admin,cap_net_raw,cap_net_bind_service+ep", binaryPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		sm.logger.Warn("Failed to set capabilities directly (systemd caps will apply)",
			"error", err,
			"output", string(output))
		return nil
	}

	sm.logger.Info("Capabilities set successfully")
	return nil
}

// MARK: CleanupNetworkResources
func (sm *ServiceManager) CleanupNetworkResources() error {
	sm.logger.Info("Cleaning up network resources")
	return nil
}

// MARK: ValidatePermissions
func (sm *ServiceManager) ValidatePermissions() error {
	if runtime.GOOS == "windows" {
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}

	binDir := filepath.Dir(execPath)
	testFile := filepath.Join(binDir, ".update-test")
	if err := os.WriteFile(testFile, []byte("test"), 0755); err != nil {
		return fmt.Errorf("cannot write to binary dir %s: %w", binDir, err)
	}
	os.Remove(testFile)

	sm.logger.Info("Permissions validated - sudo not required")
	return nil
}

// MARK: SignalShutdown
func (sm *ServiceManager) SignalShutdown() {
	sm.logger.Info("Sending shutdown signal")
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}
