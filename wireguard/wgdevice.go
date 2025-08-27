package wireguard

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/songgao/water"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

type Tunnel struct {
	name    string
	config  config.TunnelConfig
	device  *device.Device
	tunDev  *TUNDevice
	logger  *internal.Logger
	running bool
	mu      sync.RWMutex

	// Monitoring state
	stopMonitoring   chan struct{}
	monitoringActive bool
	lastError        error
	reconnectCount   map[string]int
	endpointCache    map[string]string
}

const (
	defaultMonitorInterval = 30 * time.Second
	defaultStaleTimeout    = 5 * time.Minute
	defaultKeepalive       = 25
	maxReconnectAttempts   = 5
	endpointCacheTimeout   = 10 * time.Minute
	deviceStartTimeout     = 30 * time.Second
)

// MARK: NewTunnel

// Creates a new WireGuard tunnel instance with monitoring and recovery capabilities.
func NewTunnel(cfg config.TunnelConfig, logger *internal.Logger) (*Tunnel, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("tunnel name cannot be empty")
	}

	if logger == nil {
		logger = &internal.Logger{}
	}

	return &Tunnel{
		name:           cfg.Name,
		config:         cfg,
		logger:         logger,
		stopMonitoring: make(chan struct{}),
		reconnectCount: make(map[string]int),
		endpointCache:  make(map[string]string),
	}, nil
}

// MARK: Start

// Starts the WireGuard tunnel with comprehensive error handling and monitoring.
func (t *Tunnel) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return fmt.Errorf("tunnel %s already running", t.name)
	}

	t.logger.Info("Starting tunnel", "name", t.name)

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

	t.running = true
	t.logger.Info("Tunnel started", "name", t.name, "interface", t.tunDev.Name())

	t.startMonitoring(ctx)
	return nil
}

// MARK: Stop

// Gracefully stops the tunnel and cleans up all resources.
func (t *Tunnel) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	t.logger.Info("Stopping tunnel", "name", t.name)

	t.stopMonitoringRoutine()

	ctx, cancel := context.WithTimeout(ctx, deviceStartTimeout)
	defer cancel()

	if t.device != nil {
		t.device.Close()
		t.device = nil
	}

	if t.tunDev != nil {
		t.cleanupRoutes()
		t.tunDev.Close()
		t.tunDev = nil
	}

	t.running = false
	t.lastError = nil
	t.logger.Info("Tunnel stopped", "name", t.name)
	return nil
}

// MARK: Update

// Updates tunnel configuration with rollback support on failure.
func (t *Tunnel) Update(ctx context.Context, cfg config.TunnelConfig) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if cfg.Name != t.name {
		return fmt.Errorf("cannot change tunnel name from %s to %s", t.name, cfg.Name)
	}

	oldConfig := t.config
	t.config = cfg

	if t.running {
		if err := t.applyConfiguration(); err != nil {
			t.config = oldConfig
			t.lastError = err
			return fmt.Errorf("applying updated config: %w", err)
		}

		t.logger.Info("Applied configuration update", "name", t.name)
	}

	return nil
}

// MARK: Status

// Returns the current status of the tunnel with detailed information.
func (t *Tunnel) Status(ctx context.Context) TunnelStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state := "stopped"
	if t.running {
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

// Creates and configures the TUN device for this tunnel.
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

// Adds all configured IP addresses to the TUN interface.
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

// Adds all configured routes through the TUN interface.
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

// Removes all routes when stopping the tunnel.
func (t *Tunnel) cleanupRoutes() {
	for _, route := range t.config.Routes {
		if err := t.tunDev.RemoveRoute(route); err != nil {
			t.logger.Error("Failed to remove route during cleanup", "name", t.name, "route", route, "error", err)
		}
	}
}

// MARK: createWireGuardDevice

// Creates the WireGuard device with proper logging and binding.
func (t *Tunnel) createWireGuardDevice() error {
	tunWrapper := &TUNWrapper{
		iface:  t.tunDev.File(),
		mtu:    t.config.MTU,
		name:   t.tunDev.Name(),
		events: make(chan tun.Event, 1),
	}
	tunWrapper.events <- tun.EventUp

	logLevel := device.LogLevelVerbose
	logger := device.NewLogger(logLevel, fmt.Sprintf("[%s] ", t.name))

	bind := conn.NewDefaultBind()
	t.device = device.NewDevice(tunWrapper, bind, logger)

	return nil
}

// MARK: cleanupOnFailure

// Cleans up resources when tunnel startup fails.
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

// Starts the background monitoring routine for peer connectivity.
func (t *Tunnel) startMonitoring(ctx context.Context) {
	if t.monitoringActive {
		return
	}

	t.monitoringActive = true
	go t.monitorConnections(ctx)
}

// MARK: stopMonitoringRoutine

// Stops the background monitoring routine.
func (t *Tunnel) stopMonitoringRoutine() {
	if !t.monitoringActive {
		return
	}

	t.monitoringActive = false
	close(t.stopMonitoring)
	t.stopMonitoring = make(chan struct{})
}

// MARK: monitorConnections

// Background routine that monitors peer connectivity and handles reconnection.
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

	t.logger.Info("Starting connection monitor",
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
			t.mu.RLock()
			if !t.running {
				t.mu.RUnlock()
				return
			}
			t.mu.RUnlock()

			t.performConnectivityCheck(lastHandshakes, resolvedEndpoints, staleTimeout)
		}
	}
}

// MARK: performConnectivityCheck

// Checks peer connectivity and triggers reconnection if needed.
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

// Processes WireGuard peer status and handles stale connections.
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

// Processes handshake timestamps and updates peer status.
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

// Identifies and handles stale peer connections.
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

// Safely parses WireGuard timestamp with multiple format fallbacks.
func (t *Tunnel) parseTimestamp(timestampStr string) int64 {
	if i, err := strconv.ParseInt(timestampStr, 10, 64); err == nil {
		return i
	}

	formats := []string{
		"1136239445",
		"1642782847",
		time.RFC3339,
		time.UnixDate,
	}

	for _, format := range formats {
		if timestamp, err := time.Parse(format, timestampStr); err == nil {
			return timestamp.Unix()
		}
	}

	if duration, err := time.ParseDuration(timestampStr + "s"); err == nil {
		return int64(duration.Seconds())
	}

	return 0
}

// MARK: attemptPeerRecovery

// Attempts to recover a peer connection by checking endpoint resolution.
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

// Checks if peer endpoint needs re-resolution due to IP changes.
func (t *Tunnel) checkEndpointResolution(peer config.PeerConfig, currentEndpoint, peerKey string, resolvedEndpoints map[string]string) {
	newEndpoint, err := t.resolveEndpoint(peer.Endpoint)
	if err != nil {
		t.logger.Error("Failed to re-resolve endpoint",
			"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", err)
		return
	}

	if newEndpoint != currentEndpoint {
		t.logger.Info("Endpoint IP changed, updating peer",
			"tunnel", t.name,
			"peer", peer.Name,
			"old_endpoint", currentEndpoint,
			"new_endpoint", newEndpoint)

		t.reconnectCount[peerKey]++
		t.updatePeerEndpoint(peer, newEndpoint, resolvedEndpoints)
	}
}

// MARK: attemptPeerReconnection

// Attempts to reconnect a peer by finding and updating its configuration.
func (t *Tunnel) attemptPeerReconnection(peerKey string, resolvedEndpoints map[string]string) {
	for _, peer := range t.config.Peers {
		if peerPublicKeyHex, err := t.base64ToHex(peer.PublicKey); err == nil && peerPublicKeyHex == peerKey {
			if peer.Endpoint != "" {
				newEndpoint, err := t.resolveEndpoint(peer.Endpoint)
				if err != nil {
					t.logger.Error("Failed to resolve endpoint for reconnection",
						"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", err)
					return
				}

				t.updatePeerEndpoint(peer, newEndpoint, resolvedEndpoints)
			}
			break
		}
	}
}

// MARK: updatePeerEndpoint

// Updates a peer's endpoint via WireGuard UAPI with caching.
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

// Applies WireGuard configuration to the device with validation.
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

// Builds WireGuard UAPI configuration for a single peer.
func (t *Tunnel) buildPeerConfig(peer config.PeerConfig) (string, error) {
	publicKeyHex, err := t.base64ToHex(peer.PublicKey)
	if err != nil {
		return "", fmt.Errorf("invalid public key format: %w", err)
	}

	config := fmt.Sprintf("public_key=%s\n", publicKeyHex)

	if peer.Endpoint != "" {
		resolvedEndpoint, err := t.resolveEndpoint(peer.Endpoint)
		if err != nil {
			return "", fmt.Errorf("failed to resolve endpoint %s: %w", peer.Endpoint, err)
		}
		config += fmt.Sprintf("endpoint=%s\n", resolvedEndpoint)
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

// MARK: resolveEndpoint

// Resolves hostname to IP address with caching and timeout handling.
func (t *Tunnel) resolveEndpoint(endpoint string) (string, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid endpoint format: %s", endpoint)
	}

	hostname := parts[0]
	port := parts[1]

	if net.ParseIP(hostname) != nil {
		return endpoint, nil
	}

	if cached, exists := t.endpointCache[endpoint]; exists {
		if time.Since(time.Now()) < endpointCacheTimeout {
			return cached, nil
		}
	}

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, network, address)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hostname %s: %w", hostname, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for hostname %s", hostname)
	}

	var selectedIP net.IP
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			selectedIP = ip.IP
			break
		}
	}

	if selectedIP == nil {
		selectedIP = ips[0].IP
	}

	resolved := fmt.Sprintf("%s:%s", selectedIP.String(), port)
	t.endpointCache[endpoint] = resolved

	return resolved, nil
}

// MARK: base64ToHex

// Converts base64-encoded WireGuard key to hexadecimal format.
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

// TUNWrapper adapts water.Interface to wireguard/tun.Device interface
type TUNWrapper struct {
	iface  *water.Interface
	mtu    int
	name   string
	events chan tun.Event
	closed bool
	mu     sync.Mutex
}

// MARK: File

// Returns nil as water interface doesn't expose file descriptor directly.
func (w *TUNWrapper) File() *os.File {
	return nil
}

// MARK: Read

// Reads packets from the TUN interface with proper buffer handling.
func (w *TUNWrapper) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || len(bufs) == 0 || w.iface == nil {
		return 0, net.ErrClosed
	}

	n, err := w.iface.Read(bufs[0][offset:])
	if n > 0 {
		sizes[0] = n
		return 1, nil
	}

	return 0, err
}

// MARK: Write

// Writes packets to the TUN interface with error handling.
func (w *TUNWrapper) Write(bufs [][]byte, offset int) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed || w.iface == nil {
		return 0, net.ErrClosed
	}

	for i, buf := range bufs {
		if len(buf) <= offset {
			continue
		}

		_, err := w.iface.Write(buf[offset:])
		if err != nil {
			return i, err
		}
	}

	return len(bufs), nil
}

// MARK: Flush

// Flushes any pending writes (no-op for TUN interfaces).
func (w *TUNWrapper) Flush() error {
	return nil
}

// MARK: MTU

// Returns the configured MTU size for this interface.
func (w *TUNWrapper) MTU() (int, error) {
	if w.mtu <= 0 {
		return 1420, nil
	}
	return w.mtu, nil
}

// MARK: Name

// Returns the interface name.
func (w *TUNWrapper) Name() (string, error) {
	if w.name == "" {
		return "unknown", nil
	}
	return w.name, nil
}

// MARK: Events

// Returns the events channel for interface state changes.
func (w *TUNWrapper) Events() <-chan tun.Event {
	if w.events == nil {
		w.events = make(chan tun.Event, 1)
		w.events <- tun.EventUp
	}
	return w.events
}

// MARK: Close

// Closes the TUN wrapper and underlying interface safely.
func (w *TUNWrapper) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	if w.events != nil {
		close(w.events)
		w.events = nil
	}

	if w.iface != nil {
		return w.iface.Close()
	}

	return nil
}

// MARK: BatchSize

// Returns the batch size for packet processing (single packet mode).
func (w *TUNWrapper) BatchSize() int {
	return 1
}
