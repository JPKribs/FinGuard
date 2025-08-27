package discovery

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/godbus/dbus/v5"
	"github.com/grandcat/zeroconf"
	"github.com/holoplot/go-avahi"
)

//Mark: NewDiscovery

// Creates a new mDNS service discovery manager.
func NewDiscovery(logger *internal.Logger) *Discovery {
	return &Discovery{
		logger:      logger,
		entryGroups: make(map[string]*avahi.EntryGroup),
		servers:     make(map[string]*zeroconf.Server),
		stopChan:    make(chan struct{}),
	}
}

//Mark: Start

// Initializes the mDNS publisher and determines the local IP address.
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

	d.useAvahi = d.tryAvahi()
	if !d.useAvahi {
		d.logger.Info("Avahi not available, using zeroconf fallback")
	}

	hostname, err := d.getHostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	d.hostName = hostname

	d.logger.Info("Starting mDNS publisher",
		"local_ip", localIP,
		"hostname", hostname,
		"backend", d.getBackendName())
	d.running = true

	go d.monitorServices(ctx)

	return nil
}

//Mark: tryAvahi

// Attempts to initialize Avahi, returns true if successful.
func (d *Discovery) tryAvahi() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		d.logger.Debug("D-Bus not available", "error", err)
		return false
	}

	server, err := avahi.ServerNew(conn)
	if err != nil {
		d.logger.Debug("Avahi server creation failed", "error", err)
		conn.Close()
		return false
	}

	d.conn = conn
	d.server = server
	return true
}

//Mark: getBackendName

// Returns the name of the mDNS backend being used.
func (d *Discovery) getBackendName() string {
	if d.useAvahi {
		return "avahi"
	}
	return "zeroconf"
}

//Mark: Stop

// Shuts down all published services and stops the mDNS publisher.
func (d *Discovery) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.logger.Info("Stopping mDNS publisher")

	close(d.stopChan)

	if d.useAvahi {
		for name, entryGroup := range d.entryGroups {
			if err := entryGroup.Reset(); err != nil {
				d.logger.Error("Failed to reset entry group", "name", name, "error", err)
			}
			d.logger.Debug("Stopped publishing service", "name", name)
		}

		if d.server != nil {
			d.server.Close()
		}

		if d.conn != nil {
			d.conn.Close()
		}

		d.entryGroups = make(map[string]*avahi.EntryGroup)
	} else {
		for name, server := range d.servers {
			server.Shutdown()
			d.logger.Debug("Stopped publishing service", "name", name)
		}

		d.servers = make(map[string]*zeroconf.Server)
	}

	d.running = false
	return nil
}

//Mark: getLocalIP

// Finds the best available IPv4 address for mDNS advertising.
func (d *Discovery) getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to list network interfaces: %w", err)
	}

	var candidates []string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ip := ipnet.IP.String()
				if !d.isPrivateIP(ipnet.IP) {
					return ip, nil
				}
				candidates = append(candidates, ip)
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0], nil
	}

	return "", fmt.Errorf("no suitable network interface found")
}

//Mark: getHostname

// Determines the system hostname for mDNS registration.
func (d *Discovery) getHostname() (string, error) {
	if d.useAvahi && d.server != nil {
		hostname, err := d.server.GetHostName()
		if err == nil && hostname != "" {
			return hostname, nil
		}

		hostname, err = d.server.GetHostNameFqdn()
		if err == nil && hostname != "" {
			return strings.TrimSuffix(hostname, "."), nil
		}
	}

	hostname, err := net.LookupAddr(d.localIP)
	if err == nil && len(hostname) > 0 {
		host := hostname[0]
		return strings.TrimSuffix(host, "."), nil
	}

	return "finguard-host", nil
}

//Mark: isPrivateIP

// Determines if an IP address is in a private network range.
func (d *Discovery) isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

//Mark: PublishService

// Advertises a service via mDNS with comprehensive metadata.
func (d *Discovery) PublishService(svc config.ServiceConfig, proxyPort int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !svc.PublishMDNS || !d.running {
		return nil
	}

	serviceName := strings.ToLower(svc.Name)
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	if d.useAvahi {
		return d.publishServiceAvahi(serviceName, svc, proxyPort)
	}
	return d.publishServiceZeroconf(serviceName, svc, proxyPort)
}

//Mark: publishServiceAvahi

// Publishes service using Avahi backend.
func (d *Discovery) publishServiceAvahi(serviceName string, svc config.ServiceConfig, proxyPort int) error {
	txtRecords := d.buildTXTRecords(svc)

	if existingEntryGroup, exists := d.entryGroups[serviceName]; exists {
		if err := existingEntryGroup.Reset(); err != nil {
			d.logger.Error("Failed to reset existing entry group", "name", serviceName, "error", err)
		}
		delete(d.entryGroups, serviceName)
	}

	entryGroup, err := d.server.EntryGroupNew()
	if err != nil {
		return fmt.Errorf("failed to create entry group for service %s: %w", serviceName, err)
	}

	err = entryGroup.AddService(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		serviceName,
		"_http._tcp",
		"local",
		d.hostName,
		uint16(proxyPort),
		d.convertTXTRecords(txtRecords),
	)
	if err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to add service %s: %w", serviceName, err)
	}

	if err := entryGroup.Commit(); err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to commit service %s: %w", serviceName, err)
	}

	d.entryGroups[serviceName] = entryGroup
	d.logger.Info("Published mDNS service via Avahi",
		"name", serviceName,
		"port", proxyPort,
		"host", d.hostName,
		"txt_records", len(txtRecords))

	return nil
}

//Mark: publishServiceZeroconf

// Publishes service using zeroconf backend.
func (d *Discovery) publishServiceZeroconf(serviceName string, svc config.ServiceConfig, proxyPort int) error {
	txtRecords := d.buildTXTRecords(svc)

	if existingServer, exists := d.servers[serviceName]; exists {
		existingServer.Shutdown()
	}

	server, err := zeroconf.Register(
		serviceName,
		"_http._tcp",
		"local.",
		proxyPort,
		txtRecords,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to publish service %s: %w", serviceName, err)
	}

	d.servers[serviceName] = server
	d.logger.Info("Published mDNS service via zeroconf",
		"name", serviceName,
		"port", proxyPort,
		"ip", d.localIP,
		"txt_records", len(txtRecords))

	return nil
}

//Mark: buildTXTRecords

// Creates TXT records for mDNS service advertisement.
func (d *Discovery) buildTXTRecords(svc config.ServiceConfig) []string {
	records := []string{
		fmt.Sprintf("service=%s", svc.Name),
		fmt.Sprintf("upstream=%s", svc.Upstream),
		"path=/",
	}

	if svc.Websocket {
		records = append(records, "websocket=true")
	}
	if svc.Default {
		records = append(records, "default=true")
	}
	if svc.Tunnel != "" {
		records = append(records, fmt.Sprintf("tunnel=%s", svc.Tunnel))
	}

	return records
}

//Mark: convertTXTRecords

// Converts string slice to byte slice slice for Avahi API.
func (d *Discovery) convertTXTRecords(records []string) [][]byte {
	result := make([][]byte, len(records))
	for i, record := range records {
		result[i] = []byte(record)
	}
	return result
}

//Mark: UnpublishService

// Removes a service from mDNS advertisement.
func (d *Discovery) UnpublishService(serviceName string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	serviceName = strings.ToLower(serviceName)

	if d.useAvahi {
		if entryGroup, exists := d.entryGroups[serviceName]; exists {
			if err := entryGroup.Reset(); err != nil {
				d.logger.Error("Failed to reset entry group", "name", serviceName, "error", err)
			}
			delete(d.entryGroups, serviceName)
			d.logger.Info("Unpublished mDNS service", "name", serviceName)
		}
	} else {
		if server, exists := d.servers[serviceName]; exists {
			server.Shutdown()
			delete(d.servers, serviceName)
			d.logger.Info("Unpublished mDNS service", "name", serviceName)
		}
	}
}

//Mark: ListServices

// Returns a list of currently published service names.
func (d *Discovery) ListServices() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.useAvahi {
		services := make([]string, 0, len(d.entryGroups))
		for name := range d.entryGroups {
			services = append(services, name)
		}
		return services
	} else {
		services := make([]string, 0, len(d.servers))
		for name := range d.servers {
			services = append(services, name)
		}
		return services
	}
}

//Mark: IsReady

// Reports whether the mDNS publisher is running and ready to advertise services.
func (d *Discovery) IsReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

//Mark: monitorServices

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

//Mark: healthCheck

// Performs health check on all published services.
func (d *Discovery) healthCheck() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.running {
		return
	}

	var serviceCount int
	if d.useAvahi {
		serviceCount = len(d.entryGroups)
	} else {
		serviceCount = len(d.servers)
	}

	if serviceCount > 0 {
		d.logger.Debug("mDNS services active",
			"count", serviceCount,
			"backend", d.getBackendName())
	}
}
