package utilities

import (
	"net"
)

// Public IP retrieval functions

// MARK: GetSystemIPv4s
// Retrieves all IPv4 addresses from system interfaces, prioritizing public then preferred private IPs
func GetSystemIPv4s() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var publicIPs []string
	var preferredPrivateIPs []string
	var otherIPs []string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipv4 := ipnet.IP.To4(); ipv4 != nil {
					ip := ipv4.String()

					if isPublicIP(ipv4) {
						publicIPs = append(publicIPs, ip)
					} else if isPreferredPrivateIP(ipv4) {
						preferredPrivateIPs = append(preferredPrivateIPs, ip)
					} else {
						otherIPs = append(otherIPs, ip)
					}
				}
			}
		}
	}

	allIPs := append(publicIPs, preferredPrivateIPs...)
	allIPs = append(allIPs, otherIPs...)

	if len(allIPs) == 0 {
		return nil, net.ErrClosed
	}

	return allIPs, nil
}

// MARK: GetSystemIPv6s
// Retrieves all IPv6 addresses from system interfaces, prioritizing public then preferred private IPs
func GetSystemIPv6s() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var publicIPs []string
	var preferredPrivateIPs []string
	var otherIPs []string

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipv6 := ipnet.IP.To16(); ipv6 != nil && ipnet.IP.To4() == nil { // Only IPv6, not IPv4-mapped
					ip := ipv6.String()

					if isPublicIP(ipv6) {
						publicIPs = append(publicIPs, ip)
					} else if isPreferredPrivateIP(ipv6) {
						preferredPrivateIPs = append(preferredPrivateIPs, ip)
					} else {
						otherIPs = append(otherIPs, ip)
					}
				}
			}
		}
	}

	allIPs := append(publicIPs, preferredPrivateIPs...)
	allIPs = append(allIPs, otherIPs...)

	if len(allIPs) == 0 {
		return nil, net.ErrClosed
	}

	return allIPs, nil
}

// Network interface information functions

// MARK: GetInterfaceDetails
// Retrieves detailed information about all non-loopback network interfaces
func GetInterfaceDetails() ([]NetworkInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var details []NetworkInterface

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipAddresses []string
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipv4 := ipnet.IP.To4(); ipv4 != nil {
					ipAddresses = append(ipAddresses, ipv4.String())
				}
			}
		}

		if len(ipAddresses) > 0 {
			details = append(details, NetworkInterface{
				Name:      iface.Name,
				Addresses: ipAddresses,
				IsUp:      iface.Flags&net.FlagUp != 0,
				MTU:       iface.MTU,
			})
		}
	}

	return details, nil
}

// IP classification helper functions

// MARK: isPublicIP
// Determines if an IP address is publicly routable
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}

	if ip.To4() != nil {
		privateRanges := []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16",
		}
		for _, cidr := range privateRanges {
			_, block, _ := net.ParseCIDR(cidr)
			if block.Contains(ip) {
				return false
			}
		}
	} else if ip.To16() != nil {
		privateRanges := []string{
			"fc00::/7",
			"fe80::/10",
			"::/128",
			"::1/128",
			"ff00::/8",
		}
		for _, cidr := range privateRanges {
			_, block, _ := net.ParseCIDR(cidr)
			if block.Contains(ip) {
				return false
			}
		}
	}

	return true
}

// MARK: isPreferredPrivateIP
// Checks if an IP address is in a preferred private range
func isPreferredPrivateIP(ip net.IP) bool {
	if ip.To4() != nil {
		preferredRanges := []string{
			"192.168.0.0/16",
			"10.0.0.0/8",
		}
		for _, cidr := range preferredRanges {
			_, block, _ := net.ParseCIDR(cidr)
			if block.Contains(ip) {
				return true
			}
		}
	} else if ip.To16() != nil {
		preferredRanges := []string{
			"fc00::/7",
		}
		for _, cidr := range preferredRanges {
			_, block, _ := net.ParseCIDR(cidr)
			if block.Contains(ip) {
				return true
			}
		}
	}

	return false
}
