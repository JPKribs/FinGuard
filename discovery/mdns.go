package discovery

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/grandcat/zeroconf"
)

type Discovery struct {
	logger  *internal.Logger
	servers map[string]*zeroconf.Server
	mu      sync.RWMutex
	running bool
	localIP string
}

// MARK: NewDiscovery

// Creates a new mDNS service discovery manager.
func NewDiscovery(logger *internal.Logger) *Discovery {
	return &Discovery{
		logger:  logger,
		servers: make(map[string]*zeroconf.Server),
	}
}

// MARK: Start

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

	d.logger.Info("Starting mDNS publisher", "local_ip", localIP)
	d.running = true
	return nil
}

// MARK: Stop

// Shuts down all published services and stops the mDNS publisher.
func (d *Discovery) Stop(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.logger.Info("Stopping mDNS publisher")

	for name, server := range d.servers {
		server.Shutdown()
		d.logger.Debug("Stopped publishing service", "name", name)
	}

	d.servers = make(map[string]*zeroconf.Server)
	d.running = false
	return nil
}

// MARK: getLocalIP

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
				// Prefer non-private addresses, but collect all candidates
				if !isPrivateIP(ipnet.IP) {
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

// MARK: isPrivateIP

// Determines if an IP address is in a private network range.
func isPrivateIP(ip net.IP) bool {
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

// MARK: PublishService

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

	txtRecords := d.buildTXTRecords(svc)

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

	if existingServer, exists := d.servers[serviceName]; exists {
		existingServer.Shutdown()
	}

	d.servers[serviceName] = server
	d.logger.Info("Published mDNS service",
		"name", serviceName,
		"port", proxyPort,
		"ip", d.localIP,
		"txt_records", len(txtRecords))

	return nil
}

// MARK: buildTXTRecords

// Creates TXT records for mDNS service advertisement.
func (d *Discovery) buildTXTRecords(svc config.ServiceConfig) []string {
	records := []string{
		"service=" + svc.Name,
		"upstream=" + svc.Upstream,
		"path=/",
	}

	if svc.Websocket {
		records = append(records, "websocket=true")
	}
	if svc.Default {
		records = append(records, "default=true")
	}
	if svc.Tunnel != "" {
		records = append(records, "tunnel="+svc.Tunnel)
	}

	return records
}

// MARK: UnpublishService

// Removes a service from mDNS advertisement.
func (d *Discovery) UnpublishService(serviceName string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	serviceName = strings.ToLower(serviceName)
	if server, exists := d.servers[serviceName]; exists {
		server.Shutdown()
		delete(d.servers, serviceName)
		d.logger.Info("Unpublished mDNS service", "name", serviceName)
	}
}

// MARK: ListServices

// Returns a list of currently published service names.
func (d *Discovery) ListServices() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	services := make([]string, 0, len(d.servers))
	for name := range d.servers {
		services = append(services, name)
	}
	return services
}

// MARK: IsReady

// Reports whether the mDNS publisher is running and ready to advertise services.
func (d *Discovery) IsReady() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}
