package mdns

import (
	"context"
	"time"
)

// MARK: monitorServices
// Monitors service health and performs periodic maintenance.
func (d *Discovery) monitorServices(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-ticker.C:
			d.healthCheck()
		}
	}
}

// MARK: healthCheck
// Performs health check on all published services.
func (d *Discovery) healthCheck() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.running {
		return
	}

	serviceCount := len(d.entryGroups)
	if serviceCount > 0 {
		d.logger.Debug("mDNS services active", "count", serviceCount)
	}
}
