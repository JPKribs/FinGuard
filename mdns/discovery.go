package mdns

import (
	"context"
	"fmt"
	"strings"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/holoplot/go-avahi"
)

// MARK: NewDiscovery
// Creates a new mDNS service discovery manager using Avahi only.
func NewDiscovery(logger *internal.Logger) *Discovery {
	return &Discovery{
		logger:      logger,
		entryGroups: make(map[string]*avahi.EntryGroup),
		stopChan:    make(chan struct{}),
	}
}

// MARK: Start
// Initializes the Avahi publisher and determines the local IP address.
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
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	d.hostName = hostname

	d.logger.Info("Starting mDNS publisher", "local_ip", localIP, "hostname", hostname)
	d.running = true

	go d.monitorServices(ctx)
	return nil
}

// MARK: Stop
// Shuts down all published Avahi services.
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
// Advertises a service via Avahi; skipped if Avahi unavailable.
func (d *Discovery) PublishService(svc config.ServiceConfig, proxyPort int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !svc.PublishMDNS || !d.running || d.server == nil {
		return nil
	}

	serviceName := strings.ToLower(svc.Name)
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	return d.publishServiceAvahi(serviceName, svc, proxyPort)
}

// MARK: UnpublishService
// Removes a service from Avahi advertisement.
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
// Returns a list of currently published Avahi service names.
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
// Reports whether the Avahi publisher is running.
func (d *Discovery) IsReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}
