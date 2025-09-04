package mdns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/holoplot/go-avahi"
)

// MARK: NewDiscovery
func NewDiscovery(logger *internal.Logger) *Discovery {
	return &Discovery{
		logger:      logger,
		entryGroups: make(map[string]*avahi.EntryGroup),
		stopChan:    make(chan struct{}),
	}
}

// MARK: Start
func (d *Discovery) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return fmt.Errorf("mDNS publisher already running")
	}

	localIP, err := d.getLocalIP()
	if err != nil {
		return fmt.Errorf("failed to determine local IP: %w", err)
	}
	d.localIP = localIP

	if !d.tryAvahi() {
		d.logger.Warn("Avahi Unavailable: mDNS services will be disabled")
		d.running = false
		return nil
	}

	hostname, err := d.getHostname()
	if err != nil {
		d.logger.Warn("Failed to get hostname, using fallback", "error", err)
		hostname = "finguard.local"
	}
	d.hostName = hostname

	d.logger.Info("Starting mDNS publisher", "local_ip", localIP, "hostname", hostname)
	d.running = true

	go d.monitorServices(ctx)
	return nil
}

// MARK: Stop
func (d *Discovery) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.logger.Info("Stopping mDNS publisher")
	close(d.stopChan)

	for name, entryGroup := range d.entryGroups {
		if err := entryGroup.Reset(); err != nil {
			d.logger.Error("Failed to reset entry group", "name", name, "error", err)
		}
		d.logger.Debug("Stopped publishing service", "name", name)
	}
	d.entryGroups = make(map[string]*avahi.EntryGroup)

	if d.server != nil {
		d.server.Close()
	}
	if d.conn != nil {
		d.conn.Close()
	}

	d.running = false
	return nil
}

// MARK: PublishService
func (d *Discovery) PublishService(svc config.ServiceConfig, proxyPort int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !svc.PublishMDNS || !d.running || d.server == nil {
		d.logger.Debug("Skipping mDNS publish",
			"publish_mdns", svc.PublishMDNS,
			"running", d.running,
			"server_available", d.server != nil)
		return nil
	}

	serviceName := strings.ToLower(svc.Name)
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	if err := d.validateServiceName(serviceName); err != nil {
		return fmt.Errorf("service name validation failed: %w", err)
	}

	return d.publishServiceAvahi(serviceName, svc, proxyPort)
}

// MARK: validateServiceName
func (d *Discovery) validateServiceName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("service name cannot be empty")
	}

	if len(name) > 63 {
		return fmt.Errorf("service name too long (max 63 chars): %s", name)
	}

	for i, r := range name {
		if i == 0 && !isAlphaNumeric(r) {
			return fmt.Errorf("service name must start with alphanumeric character: %s", name)
		}

		if !isAlphaNumeric(r) && r != '-' {
			return fmt.Errorf("service name contains invalid character '%c': %s", r, name)
		}

		if r == '-' && (i == 0 || i == len(name)-1) {
			return fmt.Errorf("service name cannot start or end with hyphen: %s", name)
		}
	}

	return nil
}

// MARK: UnpublishService
func (d *Discovery) UnpublishService(serviceName string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	if entryGroup, exists := d.entryGroups[serviceName]; exists {
		if err := entryGroup.Reset(); err != nil {
			d.logger.Error("Failed to reset entry group", "name", serviceName, "error", err)
		}
		delete(d.entryGroups, serviceName)
		d.logger.Info("Unpublished mDNS service", "name", serviceName)
	}
}

// MARK: ListServices
func (d *Discovery) ListServices() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	services := make([]string, 0, len(d.entryGroups))
	for name := range d.entryGroups {
		services = append(services, name)
	}
	return services
}

// MARK: IsReady
func (d *Discovery) IsReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// MARK: monitorServices
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
func (d *Discovery) healthCheck() {
	d.mu.RLock()
	serviceCount := len(d.entryGroups)
	d.mu.RUnlock()

	if serviceCount > 0 && d.server != nil {
		if state, err := d.server.GetState(); err != nil {
			d.logger.Warn("Avahi server health check failed", "error", err)
		} else {
			d.logger.Debug("Avahi server health check", "state", state, "services", serviceCount)
		}
	}
}
