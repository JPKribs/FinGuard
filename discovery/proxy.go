package discovery

import (
	"fmt"
	"strings"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/utilities"
)

func StartProxy(services []config.ServiceConfig) (*JellyfinBroadcaster, error) {
	var serverURL string
	var serviceName string

	for _, svc := range services {
		if svc.Jellyfin {
			serverURL = svc.Upstream
			serviceName = svc.Name + ".local"
			break
		}
	}

	if serverURL == "" {
		return nil, nil
	}

	serverURL = strings.TrimSuffix(serverURL, "/")

	jb := NewJellyfinBroadcaster(GetCacheDuration())

	ips, err := utilities.GetSystemIPv4s()
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("could not resolve local IPs: %w", err)
	}
	localIP := ips[0]

	hostname := "jellyfin-proxy"

	if err := jb.Start(localIP, hostname); err != nil {
		return nil, fmt.Errorf("start broadcaster: %w", err)
	}

	if err := jb.AddJellyfinService(serviceName, serverURL); err != nil {
		return nil, fmt.Errorf("add jellyfin service: %w", err)
	}

	return jb, nil
}
