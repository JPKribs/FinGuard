package discovery

import (
	"fmt"
	"log"
	"os"
	"strings"
)

func StartProxy() (*JellyfinBroadcaster, error) {
	serverURL := os.Getenv("JELLYFIN_SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8096"
		log.Println("INF: JELLYFIN_SERVER_URL not set, using default http://localhost:8096")
	}
	proxyURL := strings.TrimSuffix(os.Getenv("PROXY_URL"), "/")
	serverURL = strings.TrimSuffix(serverURL, "/")

	jb := NewJellyfinBroadcaster(GetCacheDuration())
	localIP, _ := getLocalIP()
	hostname, _ := getHostname(localIP)

	if err := jb.Start(localIP, hostname); err != nil {
		return nil, fmt.Errorf("start broadcaster: %w", err)
	}

	log.Printf("INF: Broadcaster started on %s (%s)\n", localIP, hostname)

	if err := jb.AddJellyfinService("default", serverURL); err != nil {
		return nil, fmt.Errorf("add jellyfin service: %w", err)
	}

	if proxyURL != "" {
		log.Printf("INF: Proxy mode active, using %s for Address field\n", proxyURL)
	}

	return jb, nil
}
