package config

import (
	"net/url"
	"strconv"
	"strings"
)

// MARK: GetPortFromAddr
// Extracts the port number from an address string, defaulting to 80 for http or 443 for https.
func GetPortFromAddr(addr string) int {
	if u, err := url.Parse(addr); err == nil && u.Port() != "" {
		if port, err := strconv.Atoi(u.Port()); err == nil {
			return port
		}
	}

	parts := strings.Split(addr, ":")
	if len(parts) == 2 {
		if port, err := strconv.Atoi(parts[1]); err == nil {
			return port
		}
	}

	if strings.HasPrefix(addr, "https://") || strings.HasPrefix(strings.ToLower(addr), "https:") {
		return 443
	}

	return 80
}
