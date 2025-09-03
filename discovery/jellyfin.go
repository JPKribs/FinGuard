package discovery

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/JPKribs/FinGuard/internal"
)

// MARK: NewJellyfinBroadcaster
func NewJellyfinBroadcaster(logger *internal.Logger) *JellyfinBroadcaster {
	return &JellyfinBroadcaster{
		logger:   logger,
		services: make(map[string]*JellyfinServiceInfo),
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

	jb.logger.Info("Jellyfin broadcaster started", "local_ip", localIP, "hostname", hostname)
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

	jb.logger.Info("Stopping Jellyfin broadcaster")
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

	jb.services[serviceName] = &JellyfinServiceInfo{
		Name:     serviceName,
		Upstream: upstream,
	}

	jb.logger.Info("Added Jellyfin service for broadcast", "service", serviceName, "upstream", upstream)
	return nil
}

// MARK: RemoveJellyfinService
func (jb *JellyfinBroadcaster) RemoveJellyfinService(serviceName string) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if _, exists := jb.services[serviceName]; exists {
		delete(jb.services, serviceName)
		jb.logger.Info("Removed Jellyfin service from broadcast", "service", serviceName)
	}
}

// MARK: HasJellyfinServices
func (jb *JellyfinBroadcaster) HasJellyfinServices() bool {
	jb.mu.RLock()
	defer jb.mu.RUnlock()
	return len(jb.services) > 0
}

// MARK: IsRunning
func (jb *JellyfinBroadcaster) IsRunning() bool {
	jb.mu.RLock()
	defer jb.mu.RUnlock()
	return jb.running
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
				jb.logger.Debug("UDP read error", "error", err)
				continue
			}
			return
		}

		message := string(buffer[:n])
		jb.logger.Debug("Received discovery request", "from", addr.String(), "message", message)

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

	if len(services) == 0 {
		jb.logger.Debug("No Jellyfin services to broadcast", "from", addr.String())
		return
	}

	jb.logger.Info("Handling Jellyfin discovery request", "from", addr.String(), "services", len(services))

	responseSent := false
	for _, service := range services {
		serverInfo, err := jb.fetchServerInfoFromUpstream(service.Upstream)
		if err != nil {
			jb.logger.Warn("Jellyfin service unreachable, skipping discovery response",
				"service", service.Name, "upstream", service.Upstream, "error", err)
			continue
		}

		jb.sendDiscoveryResponse(addr, service, serverInfo)
		responseSent = true
	}

	if !responseSent {
		jb.logger.Error("No Jellyfin services were reachable, no discovery responses sent", "from", addr.String())
	}
}

// MARK: sendDiscoveryResponse
func (jb *JellyfinBroadcaster) sendDiscoveryResponse(addr *net.UDPAddr, service *JellyfinServiceInfo, serverInfo *SystemInfoResponse) {
	ipResponse := JellyfinDiscoveryResponse{
		Address:         jb.localIP,
		Id:              serverInfo.Id,
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	jsonResponse, err := json.Marshal(ipResponse)
	if err != nil {
		jb.logger.Error("Failed to marshal IP response", "error", err)
		return
	}

	_, err = jb.udpConn.WriteToUDP(jsonResponse, addr)
	if err != nil {
		jb.logger.Error("Failed to send IP response", "error", err)
		return
	}

	jb.logger.Info("Sent IP discovery response", "to", addr.String(), "address", jb.localIP, "name", serverInfo.ServerName)

	time.Sleep(100 * time.Millisecond)

	serviceNameResponse := JellyfinDiscoveryResponse{
		Address:         fmt.Sprintf("%s.finguard.local", strings.ToLower(service.Name)),
		Id:              serverInfo.Id + "-svc",
		Name:            serverInfo.ServerName,
		EndpointAddress: nil,
	}

	serviceJsonResponse, err := json.Marshal(serviceNameResponse)
	if err != nil {
		jb.logger.Error("Failed to marshal service name response", "error", err)
		return
	}

	_, err = jb.udpConn.WriteToUDP(serviceJsonResponse, addr)
	if err != nil {
		jb.logger.Error("Failed to send service name response", "error", err)
		return
	}

	jb.logger.Info("Sent service name discovery response", "to", addr.String(), "address", serviceNameResponse.Address, "name", serverInfo.ServerName)
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
