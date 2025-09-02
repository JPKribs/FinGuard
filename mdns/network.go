package mdns

import (
	"fmt"
	"net"
	"strings"

	"github.com/JPKribs/FinGuard/utilities"
)

// MARK: getLocalIP
// Finds the best available IPv4 address for mDNS advertising.
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

// MARK: getHostname
// Determines the system hostname for Avahi registration.
func (d *Discovery) getHostname() (string, error) {
	if d.server != nil {
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
		return strings.TrimSuffix(hostname[0], "."), nil
	}
	return "finguard-host", nil
}
