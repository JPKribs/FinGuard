package discovery

import (
	"net"
	"sync"
	"time"
)

type JellyfinBroadcaster struct {
	localIP  string
	hostname string
	udpConn  *net.UDPConn
	services map[string]*JellyfinServiceInfo
	cache    *ServerInfoCache
	running  bool
	stopChan chan struct{}
	mu       sync.RWMutex
}

type JellyfinServiceInfo struct {
	Name        string
	Upstream    string
	ServerInfo  *SystemInfoResponse
	LastUpdated time.Time
}

type SystemInfoResponse struct {
	Id         string `json:"Id"`
	ServerName string `json:"ServerName"`
}

type JellyfinDiscoveryResponse struct {
	Address         string      `json:"Address"`
	Id              string      `json:"Id"`
	Name            string      `json:"Name"`
	EndpointAddress interface{} `json:"EndpointAddress"`
}

type ServerInfoCache struct {
	services map[string]*CachedServerInfo
	duration time.Duration
	mu       sync.RWMutex
}

type CachedServerInfo struct {
	Info      *SystemInfoResponse
	Timestamp time.Time
}
