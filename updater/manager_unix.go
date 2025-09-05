//go:build !windows

package updater

import "syscall"

func (sm *ServiceManager) SignalShutdown() {
	sm.logger.Info("Sending shutdown signal")
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
}
