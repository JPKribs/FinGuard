package wireguard

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

const (
	defaultMonitorInterval = 30 * time.Second
	defaultStaleTimeout    = 5 * time.Minute
	defaultKeepalive       = 25
	maxReconnectAttempts   = 5
	deviceStartTimeout     = 30 * time.Second
	maxBatchSize           = 32
	bufferPoolSize         = 256
)

// MARK: NewTunnel
func NewTunnel(cfg config.TunnelConfig, logger *internal.Logger, resolver *AsyncResolver) (*Tunnel, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("tunnel name cannot be empty")
	}

	if logger == nil {
		logger = &internal.Logger{}
	}

	if resolver == nil {
		resolver = NewAsyncResolver()
	}

	return &Tunnel{
		name:           cfg.Name,
		config:         cfg,
		logger:         logger,
		resolver:       resolver,
		stopMonitoring: make(chan struct{}),
		reconnectCount: make(map[string]int),
		endpointCache:  make(map[string]string),
		bufferPool:     NewPacketBufferPool(bufferPoolSize),
	}, nil
}

// MARK: Start
func (t *Tunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if atomic.LoadInt64(&t.running) == 1 {
		return fmt.Errorf("tunnel %s already running", t.name)
	}

	t.logger.Info("Starting optimized tunnel", "name", t.name)

	ctx, cancel := context.WithTimeout(ctx, deviceStartTimeout)
	defer cancel()

	if err := t.startTUNDevice(); err != nil {
		return fmt.Errorf("starting TUN device: %w", err)
	}

	if err := t.addAddresses(); err != nil {
		t.cleanupOnFailure()
		return fmt.Errorf("adding addresses: %w", err)
	}

	if err := t.createWireGuardDevice(); err != nil {
		t.cleanupOnFailure()
		return fmt.Errorf("creating WireGuard device: %w", err)
	}

	if err := t.applyConfiguration(); err != nil {
		t.cleanupOnFailure()
		return fmt.Errorf("applying WireGuard config: %w", err)
	}

	if err := t.device.Up(); err != nil {
		t.cleanupOnFailure()
		return fmt.Errorf("bringing device up: %w", err)
	}

	if err := t.addRoutes(); err != nil {
		t.logger.Error("Failed to add some routes", "name", t.name, "error", err)
	}

	atomic.StoreInt64(&t.running, 1)
	t.logger.Info("Optimized tunnel started", "name", t.name, "interface", t.tunDev.Name())

	t.startMonitoring(ctx)
	return nil
}

// MARK: Stop
func (t *Tunnel) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&t.running, 1, 0) {
		return nil
	}

	t.logger.Info("Stopping optimized tunnel", "name", t.name)

	t.stopMonitoringRoutine()

	ctx, cancel := context.WithTimeout(ctx, deviceStartTimeout)
	defer cancel()

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.device != nil {
		t.device.Close()
		t.device = nil
	}

	if t.tunDev != nil {
		t.cleanupRoutes()
		t.tunDev.Close()
		t.tunDev = nil
	}

	t.lastError = nil
	t.logger.Info("Optimized tunnel stopped", "name", t.name)
	return nil
}

// MARK: Update
func (t *Tunnel) Update(ctx context.Context, cfg config.TunnelConfig) error {
	if cfg.Name != t.name {
		return fmt.Errorf("cannot change tunnel name from %s to %s", t.name, cfg.Name)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	oldConfig := t.config
	t.config = cfg

	if atomic.LoadInt64(&t.running) == 1 {
		if err := t.applyConfiguration(); err != nil {
			t.config = oldConfig
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&t.lastError)), unsafe.Pointer(&err))
			return fmt.Errorf("applying updated config: %w", err)
		}

		t.logger.Info("Applied configuration update", "name", t.name)
	}

	return nil
}

// MARK: Status
func (t *Tunnel) Status(ctx context.Context) TunnelStatus {
	state := "stopped"
	if atomic.LoadInt64(&t.running) == 1 {
		state = "running"
	}

	ifaceName := ""
	if t.tunDev != nil {
		ifaceName = t.tunDev.Name()
	}

	status := TunnelStatus{
		Name:      t.name,
		State:     state,
		Interface: ifaceName,
		MTU:       t.config.MTU,
		Peers:     len(t.config.Peers),
	}

	if t.lastError != nil {
		status.Error = t.lastError.Error()
	}

	return status
}

// MARK: startTUNDevice
func (t *Tunnel) startTUNDevice() error {
	mtu := t.config.MTU
	if mtu <= 0 {
		mtu = 1420
	}

	tunDev, err := CreateTUN(t.name, mtu)
	if err != nil {
		return fmt.Errorf("creating TUN device: %w", err)
	}

	t.tunDev = tunDev
	return nil
}

// MARK: addAddresses
func (t *Tunnel) addAddresses() error {
	if len(t.config.Addresses) == 0 {
		return fmt.Errorf("no addresses configured for tunnel %s", t.name)
	}

	for _, addr := range t.config.Addresses {
		if err := t.tunDev.AddAddress(addr); err != nil {
			return fmt.Errorf("adding address %s: %w", addr, err)
		}
		t.logger.Info("Added address to tunnel", "name", t.name, "address", addr)
	}

	return nil
}

// MARK: addRoutes
func (t *Tunnel) addRoutes() error {
	var errors []string

	for _, route := range t.config.Routes {
		if err := t.tunDev.AddRoute(route); err != nil {
			errors = append(errors, fmt.Sprintf("route %s: %v", route, err))
			t.logger.Error("Failed to add route", "name", t.name, "route", route, "error", err)
		} else {
			t.logger.Info("Added route to tunnel", "name", t.name, "route", route)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to add routes: %s", strings.Join(errors, "; "))
	}

	return nil
}

// MARK: cleanupRoutes
func (t *Tunnel) cleanupRoutes() {
	for _, route := range t.config.Routes {
		if err := t.tunDev.RemoveRoute(route); err != nil {
			t.logger.Error("Failed to remove route during cleanup", "name", t.name, "route", route, "error", err)
		}
	}
}

// MARK: createWireGuardDevice
func (t *Tunnel) createWireGuardDevice() error {
	tunWrapper := &OptimizedTUNWrapper{
		iface:      t.tunDev.File(),
		mtu:        t.config.MTU,
		name:       t.tunDev.Name(),
		events:     make(chan tun.Event, 1),
		bufferPool: t.bufferPool,
		batchSize:  maxBatchSize,
	}
	tunWrapper.events <- tun.EventUp

	logLevel := device.LogLevelError
	logger := device.NewLogger(logLevel, fmt.Sprintf("[%s] ", t.name))

	bind := conn.NewDefaultBind()
	t.device = device.NewDevice(tunWrapper, bind, logger)

	return nil
}

// MARK: cleanupOnFailure
func (t *Tunnel) cleanupOnFailure() {
	if t.device != nil {
		t.device.Close()
		t.device = nil
	}

	if t.tunDev != nil {
		t.tunDev.Close()
		t.tunDev = nil
	}
}

// MARK: startMonitoring
func (t *Tunnel) startMonitoring(ctx context.Context) {
	if atomic.LoadInt64(&t.monitoringActive) == 1 {
		return
	}

	atomic.StoreInt64(&t.monitoringActive, 1)
	go t.monitorConnections(ctx)
}

// MARK: stopMonitoringRoutine
func (t *Tunnel) stopMonitoringRoutine() {
	if !atomic.CompareAndSwapInt64(&t.monitoringActive, 1, 0) {
		return
	}

	select {
	case <-t.stopMonitoring:
	default:
		close(t.stopMonitoring)
	}

	t.stopMonitoring = make(chan struct{})
}

// MARK: monitorConnections
func (t *Tunnel) monitorConnections(ctx context.Context) {
	monitorInterval := time.Duration(t.config.MonitorInterval) * time.Second
	if monitorInterval <= 0 {
		monitorInterval = defaultMonitorInterval
	}

	staleTimeout := time.Duration(t.config.StaleConnectionTimeout) * time.Second
	if staleTimeout <= 0 {
		staleTimeout = defaultStaleTimeout
	}

	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	lastHandshakes := make(map[string]time.Time)
	resolvedEndpoints := make(map[string]string)

	t.logger.Info("Starting optimized connection monitor",
		"tunnel", t.name,
		"interval", monitorInterval,
		"stale_timeout", staleTimeout)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopMonitoring:
			return
		case <-ticker.C:
			if atomic.LoadInt64(&t.running) == 0 {
				return
			}
			t.performConnectivityCheck(lastHandshakes, resolvedEndpoints, staleTimeout)
		}
	}
}

// MARK: performConnectivityCheck
func (t *Tunnel) performConnectivityCheck(lastHandshakes map[string]time.Time, resolvedEndpoints map[string]string, staleTimeout time.Duration) {
	if t.device == nil {
		return
	}

	var statusBuf strings.Builder
	if err := t.device.IpcGetOperation(&statusBuf); err != nil {
		t.logger.Error("Failed to get tunnel status", "name", t.name, "error", err)
		t.lastError = err
		return
	}

	t.processPeerStatus(statusBuf.String(), lastHandshakes, resolvedEndpoints, staleTimeout)
}

// MARK: processPeerStatus
func (t *Tunnel) processPeerStatus(status string, lastHandshakes map[string]time.Time, resolvedEndpoints map[string]string, staleTimeout time.Duration) {
	lines := strings.Split(status, "\n")
	var currentPeer, currentEndpoint string
	activePeers := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "public_key="):
			currentPeer = strings.TrimPrefix(line, "public_key=")
		case strings.HasPrefix(line, "endpoint="):
			currentEndpoint = strings.TrimPrefix(line, "endpoint=")
		case strings.HasPrefix(line, "last_handshake_time_sec=") && currentPeer != "":
			t.processHandshakeTime(line, currentPeer, currentEndpoint, lastHandshakes, activePeers, resolvedEndpoints)
		}
	}

	t.checkStaleConnections(lastHandshakes, activePeers, resolvedEndpoints, staleTimeout)
}

// MARK: processHandshakeTime
func (t *Tunnel) processHandshakeTime(line, currentPeer, currentEndpoint string, lastHandshakes map[string]time.Time, activePeers map[string]bool, resolvedEndpoints map[string]string) {
	timestampStr := strings.TrimPrefix(line, "last_handshake_time_sec=")

	if timestampStr != "0" {
		if timestamp := t.parseTimestamp(timestampStr); timestamp > 0 {
			handshakeTime := time.Unix(timestamp, 0)
			lastHandshakes[currentPeer] = handshakeTime
			activePeers[currentPeer] = true

			delete(t.reconnectCount, currentPeer)

			t.logger.Debug("Peer active", "tunnel", t.name, "peer", currentPeer[:8]+"...", "last_handshake", handshakeTime.Format(time.RFC3339))
		}
	} else {
		t.attemptPeerRecovery(currentPeer, currentEndpoint, lastHandshakes, resolvedEndpoints)
	}
}

// MARK: checkStaleConnections
func (t *Tunnel) checkStaleConnections(lastHandshakes map[string]time.Time, activePeers map[string]bool, resolvedEndpoints map[string]string, staleTimeout time.Duration) {
	staleThreshold := time.Now().Add(-staleTimeout)

	for peerKey, lastHandshake := range lastHandshakes {
		if !activePeers[peerKey] && lastHandshake.Before(staleThreshold) {
			attempts := t.reconnectCount[peerKey]
			maxRetries := t.config.ReconnectionRetries
			if maxRetries <= 0 {
				maxRetries = maxReconnectAttempts
			}

			if attempts < maxRetries {
				t.logger.Info("Peer connection stale, attempting reconnection",
					"tunnel", t.name,
					"peer", peerKey[:8]+"...",
					"attempt", attempts+1,
					"max_retries", maxRetries,
					"last_handshake", lastHandshake.Format(time.RFC3339))

				t.reconnectCount[peerKey] = attempts + 1
				t.attemptPeerReconnection(peerKey, resolvedEndpoints)
			} else {
				t.logger.Error("Peer reconnection failed after maximum retries",
					"tunnel", t.name,
					"peer", peerKey[:8]+"...",
					"attempts", attempts)
			}
		}
	}
}

// MARK: parseTimestamp
func (t *Tunnel) parseTimestamp(timestampStr string) int64 {
	timestamp := int64(0)
	for _, c := range []byte(timestampStr) {
		if c >= '0' && c <= '9' {
			timestamp = timestamp*10 + int64(c-'0')
		} else {
			return 0
		}
	}

	if timestamp > 1000000000 && timestamp < 4000000000 {
		return timestamp
	}

	return 0
}

// MARK: attemptPeerRecovery
func (t *Tunnel) attemptPeerRecovery(peerKey, currentEndpoint string, lastHandshakes map[string]time.Time, resolvedEndpoints map[string]string) {
	if lastHandshake, exists := lastHandshakes[peerKey]; exists {
		if time.Since(lastHandshake) < 2*time.Minute {
			return
		}
	}

	for _, peer := range t.config.Peers {
		if peerPublicKeyHex, err := t.base64ToHex(peer.PublicKey); err == nil && peerPublicKeyHex == peerKey {
			if peer.Endpoint != "" && currentEndpoint != "" {
				t.checkEndpointResolution(peer, currentEndpoint, peerKey, resolvedEndpoints)
			}
			break
		}
	}
}

// MARK: checkEndpointResolution
func (t *Tunnel) checkEndpointResolution(peer config.PeerConfig, currentEndpoint, peerKey string, resolvedEndpoints map[string]string) {
	resultChan := t.resolver.ResolveAsync(peer.Endpoint, 5*time.Second)

	select {
	case result := <-resultChan:
		if result.err != nil {
			t.logger.Error("Failed to re-resolve endpoint",
				"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", result.err)
			return
		}

		if result.endpoint != currentEndpoint {
			t.logger.Info("Endpoint IP changed, updating peer",
				"tunnel", t.name,
				"peer", peer.Name,
				"old_endpoint", currentEndpoint,
				"new_endpoint", result.endpoint)

			t.reconnectCount[peerKey]++
			t.updatePeerEndpoint(peer, result.endpoint, resolvedEndpoints)
		}
	case <-time.After(10 * time.Second):
		t.logger.Warn("Endpoint resolution timed out", "tunnel", t.name, "peer", peer.Name)
	}
}

// MARK: attemptPeerReconnection
func (t *Tunnel) attemptPeerReconnection(peerKey string, resolvedEndpoints map[string]string) {
	for _, peer := range t.config.Peers {
		if peerPublicKeyHex, err := t.base64ToHex(peer.PublicKey); err == nil && peerPublicKeyHex == peerKey {
			if peer.Endpoint != "" {
				resultChan := t.resolver.ResolveAsync(peer.Endpoint, 5*time.Second)

				select {
				case result := <-resultChan:
					if result.err != nil {
						t.logger.Error("Failed to resolve endpoint for reconnection",
							"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", result.err)
						return
					}

					t.updatePeerEndpoint(peer, result.endpoint, resolvedEndpoints)
				case <-time.After(10 * time.Second):
					t.logger.Warn("Endpoint resolution timeout during reconnection", "tunnel", t.name, "peer", peer.Name)
				}
			}
			break
		}
	}
}

// MARK: updatePeerEndpoint
func (t *Tunnel) updatePeerEndpoint(peer config.PeerConfig, newEndpoint string, resolvedEndpoints map[string]string) {
	peerKey := fmt.Sprintf("%s:%s", t.name, peer.Name)

	if lastResolved, exists := resolvedEndpoints[peerKey]; exists && lastResolved == newEndpoint {
		return
	}

	resolvedEndpoints[peerKey] = newEndpoint

	publicKeyHex, err := t.base64ToHex(peer.PublicKey)
	if err != nil {
		t.logger.Error("Invalid public key for peer endpoint update",
			"tunnel", t.name, "peer", peer.Name, "error", err)
		return
	}

	uapi := fmt.Sprintf("public_key=%s\nendpoint=%s\n", publicKeyHex, newEndpoint)

	if err := t.device.IpcSetOperation(strings.NewReader(uapi)); err != nil {
		t.logger.Error("Failed to update peer endpoint",
			"tunnel", t.name, "peer", peer.Name, "endpoint", newEndpoint, "error", err)
		t.lastError = err
	} else {
		t.logger.Info("Updated peer endpoint",
			"tunnel", t.name, "peer", peer.Name, "new_endpoint", newEndpoint)
	}
}

// MARK: applyConfiguration
func (t *Tunnel) applyConfiguration() error {
	if t.device == nil {
		return fmt.Errorf("device not initialized")
	}

	privateKey := strings.TrimSpace(t.config.PrivateKey)
	if privateKey == "" {
		return fmt.Errorf("private_key must be provided in tunnel config")
	}

	privateKeyHex, err := t.base64ToHex(privateKey)
	if err != nil {
		return fmt.Errorf("invalid private key format: %w", err)
	}

	uapi := fmt.Sprintf("private_key=%s\n", privateKeyHex)

	if t.config.ListenPort > 0 {
		if t.config.ListenPort < 1 || t.config.ListenPort > 65535 {
			return fmt.Errorf("invalid listen port: %d", t.config.ListenPort)
		}
		uapi += fmt.Sprintf("listen_port=%d\n", t.config.ListenPort)
	}

	for _, peer := range t.config.Peers {
		peerConfig, err := t.buildPeerConfig(peer)
		if err != nil {
			return fmt.Errorf("building config for peer %s: %w", peer.Name, err)
		}
		uapi += peerConfig
	}

	return t.device.IpcSetOperation(strings.NewReader(uapi))
}

// MARK: buildPeerConfig
func (t *Tunnel) buildPeerConfig(peer config.PeerConfig) (string, error) {
	publicKeyHex, err := t.base64ToHex(peer.PublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid public key format: %w", err)
	}

	config := fmt.Sprintf("public_key=%s\n", publicKeyHex)

	if peer.Endpoint != "" {
		if resolved, ok := t.resolver.ResolveFast(peer.Endpoint); ok {
			config += fmt.Sprintf("endpoint=%s\n", resolved)
		} else {
			resultChan := t.resolver.ResolveAsync(peer.Endpoint, 5*time.Second)
			select {
			case result := <-resultChan:
				if result.err == nil {
					config += fmt.Sprintf("endpoint=%s\n", result.endpoint)
				}
			case <-time.After(5 * time.Second):
				config += fmt.Sprintf("endpoint=%s\n", peer.Endpoint)
			}
		}
	}

	if peer.Preshared != "" {
		presharedHex, err := t.base64ToHex(peer.Preshared)
		if err != nil {
			return "", fmt.Errorf("invalid preshared key format: %w", err)
		}
		config += fmt.Sprintf("preshared_key=%s\n", presharedHex)
	}

	if len(peer.AllowedIPs) == 0 {
		return "", fmt.Errorf("peer %s must have at least one allowed IP", peer.Name)
	}

	for _, allowedIP := range peer.AllowedIPs {
		if _, _, err := net.ParseCIDR(allowedIP); err != nil {
			return "", fmt.Errorf("invalid allowed IP %s: %w", allowedIP, err)
		}
		config += fmt.Sprintf("allowed_ip=%s\n", allowedIP)
	}

	if peer.Persistent || peer.PersistentKeepaliveInt > 0 {
		keepalive := peer.PersistentKeepaliveInt
		if keepalive <= 0 {
			keepalive = defaultKeepalive
		}
		config += fmt.Sprintf("persistent_keepalive_interval=%d\n", keepalive)
	}

	return config, nil
}

// MARK: base64ToHex
func (t *Tunnel) base64ToHex(b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if len(b64) != 44 {
		return "", fmt.Errorf("key must be 44 characters, got %d", len(b64))
	}

	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64 decode error: %w", err)
	}

	if len(decoded) != 32 {
		return "", fmt.Errorf("key must decode to 32 bytes, got %d", len(decoded))
	}

	return fmt.Sprintf("%x", decoded), nil
}
