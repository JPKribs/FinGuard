package discovery

import (
	"net"
	"sync"

	"github.com/JPKribs/FinGuard/internal"
)

// MARK: JellyfinBroadcaster
type JellyfinBroadcaster struct {
	logger   *internal.Logger
	localIP  string
	hostname string
	udpConn  *net.UDPConn
	services map[string]*JellyfinServiceInfo
	running  bool
	stopChan chan struct{}
	mu       sync.RWMutex
}

// MARK: JellyfinServiceInfo
type JellyfinServiceInfo struct {
	Name     string
	Upstream string
}

// MARK: SystemInfoResponse
type SystemInfoResponse struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

// MARK: JellyfinDiscoveryResponse
type JellyfinDiscoveryResponse struct {
	Address         string      `json:"Address"`
	Id              string      `json:"Id"`
	Name            string      `json:"Name"`
	EndpointAddress interface{} `json:"EndpointAddress"`
}
