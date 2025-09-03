package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// MARK: NewJellyfinBroadcaster
func NewJellyfinBroadcaster(cacheDuration time.Duration) *JellyfinBroadcaster {
	return &JellyfinBroadcaster{
		services: make(map[string]*JellyfinServiceInfo),
		cache:    NewServerInfoCache(cacheDuration),
		stopChan: make(chan struct{}),
	}
}

// MARK: Start
func (jb *JellyfinBroadcaster) Start(localIP, hostname string) error {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.running {
		return fmt.Errorf("broadcaster already running")
	}

	jb.localIP = localIP
	jb.hostname = hostname

	addr, err := net.ResolveUDPAddr("udp", ":7359")
	if err != nil {
		return fmt.Errorf("error resolving UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("error listening on UDP port 7359: %w", err)
	}

	jb.udpConn = conn
	jb.running = true

	go jb.handleDiscoveryRequests()
	return nil
}

// MARK: Stop
func (jb *JellyfinBroadcaster) Stop() error {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if !jb.running {
		return nil
	}

	close(jb.stopChan)
	if jb.udpConn != nil {
		jb.udpConn.Close()
	}

	jb.running = false
	return nil
}

// MARK: AddJellyfinService
func (jb *JellyfinBroadcaster) AddJellyfinService(serviceName, upstream string) error {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	serverInfo, err := jb.fetchServerInfoFromUpstream(upstream)
	if err != nil {
		return fmt.Errorf("failed to fetch server info from %s: %w", upstream, err)
	}

	jb.services[serviceName] = &JellyfinServiceInfo{
		Name:        serviceName,
		Upstream:    upstream,
		ServerInfo:  serverInfo,
		LastUpdated: time.Now(),
	}

	jb.cache.Set(serviceName, serverInfo)
	return nil
}

// MARK: RemoveJellyfinService
func (jb *JellyfinBroadcaster) RemoveJellyfinService(serviceName string) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	delete(jb.services, serviceName)
	jb.cache.Remove(serviceName)
}

// MARK: handleDiscoveryRequests
func (jb *JellyfinBroadcaster) handleDiscoveryRequests() {
	buffer := make([]byte, 1024)

	for {
		select {
		case <-jb.stopChan:
			return
		default:
		}

		n, addr, err := jb.udpConn.ReadFromUDP(buffer)
		if err != nil {
			if jb.running {
				continue
			}
			return
		}

		message := string(buffer[:n])
		if strings.EqualFold(message, "Who is JellyfinServer?") {
			go jb.handleDiscoveryRequest(addr)
		}
	}
}

// MARK: handleDiscoveryRequest
func (jb *JellyfinBroadcaster) handleDiscoveryRequest(addr *net.UDPAddr) {
	jb.mu.RLock()
	services := make(map[string]*JellyfinServiceInfo)
	for name, svc := range jb.services {
		services[name] = svc
	}
	jb.mu.RUnlock()

	for serviceName, service := range services {
		serverInfo := jb.cache.Get(serviceName)
		if serverInfo == nil {
			freshInfo, err := jb.fetchServerInfoFromUpstream(service.Upstream)
			if err != nil {
				continue
			}
			serverInfo = freshInfo
			jb.cache.Set(serviceName, serverInfo)
		}

		jb.sendDiscoveryResponse(addr, service, serverInfo)
	}
}

// MARK: sendDiscoveryResponse
func (jb *JellyfinBroadcaster) sendDiscoveryResponse(addr *net.UDPAddr, service *JellyfinServiceInfo, serverInfo *SystemInfoResponse) {
	response := JellyfinDiscoveryResponse{
		Address:         jb.localIP,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return
	}

	jb.udpConn.WriteToUDP(jsonResponse, addr)

	serviceNameResponse := JellyfinDiscoveryResponse{
		Address:         fmt.Sprintf("%s.local", strings.ToLower(service.Name)),
		Id:              serverInfo.Id + "-svc",
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	serviceJsonResponse, err := json.Marshal(serviceNameResponse)
	if err != nil {
		return
	}

	time.Sleep(100 * time.Millisecond)
	jb.udpConn.WriteToUDP(serviceJsonResponse, addr)
}

// MARK: fetchServerInfoFromUpstream
func (jb *JellyfinBroadcaster) fetchServerInfoFromUpstream(upstream string) (*SystemInfoResponse, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	infoURL := fmt.Sprintf("%s/System/Info/Public", strings.TrimSuffix(upstream, "/"))
	resp, err := client.Get(infoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request returned status: %d", resp.StatusCode)
	}

	var serverInfo SystemInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&serverInfo)
	if err != nil {
		return nil, err
	}

	return &serverInfo, nil
}

// MARK: IsRunning
func (jb *JellyfinBroadcaster) IsRunning() bool {
	jb.mu.RLock()
	defer jb.mu.RUnlock()
	return jb.running
}
