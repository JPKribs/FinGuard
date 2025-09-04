package mdns

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/utilities"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// MARK: tryAvahi
func (d *Discovery) tryAvahi() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	if err := d.verifyAvahiDaemon(); err != nil {
		d.logger.Debug("Avahi daemon check failed", "error", err)
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

// MARK: verifyAvahiDaemon
func (d *Discovery) verifyAvahiDaemon() error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("D-Bus system bus not available: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.Avahi", "/")
	call := obj.Call("org.freedesktop.DBus.Peer.Ping", 0)
	if call.Err != nil {
		return fmt.Errorf("avahi daemon not responding: %w", call.Err)
	}

	return nil
}

// MARK: sanitizeServiceName
func (d *Discovery) sanitizeServiceName(name string) string {
	name = strings.ToLower(name)

	var result strings.Builder
	for i, r := range name {
		if isAlphaNumeric(r) {
			result.WriteRune(r)
		} else if r == '-' && i > 0 && i < len(name)-1 {
			result.WriteRune(r)
		} else if !isAlphaNumeric(r) && result.Len() > 0 {
			if result.Len() > 0 && result.String()[result.Len()-1] != '-' {
				result.WriteRune('-')
			}
		}
	}

	sanitized := strings.Trim(result.String(), "-")
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
		sanitized = strings.TrimRight(sanitized, "-")
	}

	if len(sanitized) == 0 {
		sanitized = "service"
	}

	return sanitized
}

// MARK: isAlphaNumeric
func isAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// MARK: getHostname
func (d *Discovery) getHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		d.logger.Warn("Failed to get system hostname, using fallback", "error", err)
		hostname = "finguard"
	}

	hostname = d.sanitizeHostname(hostname)

	if hostname == "" {
		hostname = "finguard"
	}

	if !strings.HasSuffix(hostname, ".local") {
		hostname = fmt.Sprintf("%s.local", hostname)
	}

	return hostname, nil
}

// MARK: sanitizeHostname
func (d *Discovery) sanitizeHostname(hostname string) string {
	hostname = strings.ToLower(hostname)
	hostname = strings.TrimSuffix(hostname, ".local")

	var result strings.Builder
	for i, r := range hostname {
		if isAlphaNumeric(r) {
			result.WriteRune(r)
		} else if r == '-' && i > 0 && i < len(hostname)-1 {
			result.WriteRune(r)
		} else if r == '.' && i > 0 && i < len(hostname)-1 {
			result.WriteRune(r)
		}
	}

	sanitized := strings.Trim(result.String(), "-.")
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		sanitized = strings.TrimRight(sanitized, "-.")
	}

	return sanitized
}

// MARK: publishServiceAvahi
func (d *Discovery) publishServiceAvahi(serviceName string, svc config.ServiceConfig, proxyPort int) error {
	sanitizedName := d.sanitizeServiceName(serviceName)
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

	// Publish the main service
	fullServiceName := fmt.Sprintf("%s FinGuard", sanitizedName)
	err = entryGroup.AddService(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		fullServiceName,
		"_http._tcp",
		"local",
		d.hostName,
		uint16(proxyPort),
		d.convertTXTRecords(txtRecords),
	)
	if err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to add service %s (sanitized: %s, full: %s): %w",
			serviceName, sanitizedName, fullServiceName, err)
	}

	// Add CNAME record for subdomain access (servicename.hostname.local -> hostname.local)
	baseHostname := strings.TrimSuffix(d.hostName, ".local")
	subdomainName := fmt.Sprintf("%s.%s.local", sanitizedName, baseHostname)

	// Use raw values since constants aren't exposed
	// RecordClassIn = 1, RecordTypeCName = 5
	err = entryGroup.AddRecord(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		subdomainName,
		1,   // CLASS_IN
		5,   // TYPE_CNAME
		300, // TTL
		[]byte(d.hostName),
	)
	if err != nil {
		d.logger.Warn("Failed to add CNAME record for subdomain",
			"subdomain", subdomainName, "target", d.hostName, "error", err)
		// Don't fail the entire operation if CNAME fails
	} else {
		d.logger.Debug("Added CNAME record", "subdomain", subdomainName, "target", d.hostName)
	}

	if err := entryGroup.Commit(); err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to commit service %s: %w", serviceName, err)
	}

	d.entryGroups[serviceName] = entryGroup
	d.logger.Info("Published mDNS service via Avahi",
		"original_name", serviceName,
		"sanitized_name", sanitizedName,
		"full_name", fullServiceName,
		"subdomain", subdomainName,
		"port", proxyPort,
		"host", d.hostName,
		"txt_records", len(txtRecords))
	return nil
}

// MARK: buildTXTRecords
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

// MARK: convertTXTRecords
func (d *Discovery) convertTXTRecords(records []string) [][]byte {
	result := make([][]byte, len(records))
	for i, record := range records {
		result[i] = []byte(record)
	}
	return result
}

// MARK: getLocalIP
func (d *Discovery) getLocalIP() (string, error) {
	ipv4List, err := utilities.GetSystemIPv4s()
	if err != nil {
		return "", fmt.Errorf("failed to get system IPv4 addresses: %w", err)
	}

	if len(ipv4List) == 0 {
		return "", fmt.Errorf("no suitable network interface found")
	}

	return ipv4List[0], nil
}
