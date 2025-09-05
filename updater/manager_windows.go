//go:build windows

package updater

import "os"

func (sm *ServiceManager) SignalShutdown() {
	sm.logger.Info("Shutdown requested (Windows)")
	p, err := os.FindProcess(os.Getpid())
	if err == nil {
		p.Signal(os.Kill)
	}
}
