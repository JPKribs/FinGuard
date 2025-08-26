package wireguard

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
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
}

// MARK: Create new tunnel
func NewTunnel(cfg config.TunnelConfig, logger *internal.Logger) (*Tunnel, error) {
	return &Tunnel{
		name:   cfg.Name,
		config: cfg,
		logger: logger,
	}, nil
}

// MARK: Start tunnel
func (t *Tunnel) Start(ctx context.Context) error {
	if t.running {
		return fmt.Errorf("tunnel %s already running", t.name)
	}

	t.logger.Info("Starting tunnel", "name", t.name)

	tunDev, err := CreateTUN(t.name, t.config.MTU)
	if err != nil {
		return fmt.Errorf("creating TUN device: %w", err)
	}
	t.tunDev = tunDev

	for _, addr := range t.config.Addresses {
		if err := tunDev.AddAddress(addr); err != nil {
			tunDev.Close()
			return fmt.Errorf("adding address %s: %w", addr, err)
		}
		t.logger.Info("Added address to tunnel", "name", t.name, "address", addr)
	}

	tunWrapper := &TUNWrapper{
		iface:  tunDev.File(),
		mtu:    t.config.MTU,
		name:   tunDev.Name(),
		events: make(chan tun.Event, 1),
	}
	tunWrapper.events <- tun.EventUp

	logLevel := device.LogLevelVerbose
	logger := device.NewLogger(logLevel, fmt.Sprintf("[%s] ", t.name))

	bind := conn.NewDefaultBind()
	dev := device.NewDevice(tunWrapper, bind, logger)
	t.device = dev

	if err := t.applyConfig(); err != nil {
		dev.Close()
		tunDev.Close()
		return fmt.Errorf("applying WireGuard config: %w", err)
	}

	if err := dev.Up(); err != nil {
		dev.Close()
		tunDev.Close()
		return fmt.Errorf("bringing device up: %w", err)
	}

	for _, route := range t.config.Routes {
		if err := tunDev.AddRoute(route); err != nil {
			t.logger.Error("Failed to add route", "name", t.name, "route", route, "error", err)
		} else {
			t.logger.Info("Added route to tunnel", "name", t.name, "route", route)
		}
	}

	t.running = true
	t.logger.Info("Tunnel started", "name", t.name, "interface", tunDev.Name())

	go t.monitorConnections(ctx)

	return nil
}

// MARK: Stop tunnel
func (t *Tunnel) Stop(ctx context.Context) error {
	if !t.running {
		return nil
	}

	t.logger.Info("Stopping tunnel", "name", t.name)

	if t.device != nil {
		t.device.Close()
	}

	if t.tunDev != nil {
		t.tunDev.Close()
	}

	t.running = false
	t.logger.Info("Tunnel stopped", "name", t.name)
	return nil
}

// MARK: Update tunnel configuration
func (t *Tunnel) Update(ctx context.Context, cfg config.TunnelConfig) error {
	t.config = cfg
	if t.running {
		return t.applyConfig()
	}
	return nil
}

// MARK: Get tunnel status
func (t *Tunnel) Status(ctx context.Context) TunnelStatus {
	state := "stopped"
	if t.running {
		state = "running"
	}

	ifaceName := ""
	if t.tunDev != nil {
		ifaceName = t.tunDev.Name()
	}

	return TunnelStatus{
		Name:      t.name,
		State:     state,
		Interface: ifaceName,
		MTU:       t.config.MTU,
		Peers:     len(t.config.Peers),
	}
}

// MARK: Monitor peer connections
func (t *Tunnel) monitorConnections(ctx context.Context) {
	// Configure monitoring intervals
	monitorInterval := time.Duration(t.config.MonitorInterval) * time.Second
	if monitorInterval == 0 {
		monitorInterval = 30 * time.Second // Default 30s
	}

	staleTimeout := time.Duration(t.config.StaleConnectionTimeout) * time.Second
	if staleTimeout == 0 {
		staleTimeout = 5 * time.Minute // Default 5min
	}

	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()

	// Track last handshake times and resolved endpoints
	lastHandshakes := make(map[string]time.Time)
	resolvedEndpoints := make(map[string]string)
	reconnectionAttempts := make(map[string]int)

	t.logger.Info("Starting connection monitor",
		"tunnel", t.name,
		"interval", monitorInterval,
		"stale_timeout", staleTimeout)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !t.running {
				return
			}
			t.checkPeerConnectivity(lastHandshakes, resolvedEndpoints, reconnectionAttempts, staleTimeout)
		}
	}
}

// MARK: Check peer connectivity with reconnection logic
func (t *Tunnel) checkPeerConnectivity(lastHandshakes map[string]time.Time, resolvedEndpoints map[string]string, reconnectionAttempts map[string]int, staleTimeout time.Duration) {
	if t.device == nil {
		return
	}

	var statusBuf strings.Builder
	if err := t.device.IpcGetOperation(&statusBuf); err != nil {
		t.logger.Error("Failed to get tunnel status", "name", t.name, "error", err)
		return
	}

	status := statusBuf.String()
	lines := strings.Split(status, "\n")
	var currentPeer string
	var currentEndpoint string
	activePeers := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "public_key=") {
			currentPeer = strings.TrimPrefix(line, "public_key=")
		} else if strings.HasPrefix(line, "endpoint=") {
			currentEndpoint = strings.TrimPrefix(line, "endpoint=")
		} else if strings.HasPrefix(line, "last_handshake_time_sec=") && currentPeer != "" {
			timestampStr := strings.TrimPrefix(line, "last_handshake_time_sec=")

			if timestampStr != "0" {
				timestamp := time.Unix(parseTimestamp(timestampStr), 0)
				lastHandshakes[currentPeer] = timestamp
				activePeers[currentPeer] = true

				// Reset reconnection attempts on successful handshake
				delete(reconnectionAttempts, currentPeer)

				t.logger.Debug("Peer active", "tunnel", t.name, "peer", currentPeer[:8]+"...", "last_handshake", timestamp.Format(time.RFC3339))
			} else {
				// No handshake - check if we need to re-resolve
				t.checkPeerReconnection(currentPeer, currentEndpoint, lastHandshakes, resolvedEndpoints, reconnectionAttempts)
			}
		}
	}

	// Check for stale connections
	staleThreshold := time.Now().Add(-staleTimeout)
	for peerKey, lastHandshake := range lastHandshakes {
		if !activePeers[peerKey] && lastHandshake.Before(staleThreshold) {
			attempts := reconnectionAttempts[peerKey]
			maxRetries := t.config.ReconnectionRetries
			if maxRetries == 0 {
				maxRetries = 3 // Default
			}

			if attempts < maxRetries {
				t.logger.Info("Peer connection appears stale, attempting reconnection",
					"tunnel", t.name,
					"peer", peerKey[:8]+"...",
					"attempt", attempts+1,
					"max_retries", maxRetries,
					"last_handshake", lastHandshake.Format(time.RFC3339))

				// Find the peer config and attempt re-resolution
				for _, peer := range t.config.Peers {
					if peerPublicKeyHex, err := base64ToHex(peer.PublicKey); err == nil {
						if peerPublicKeyHex == peerKey {
							reconnectionAttempts[peerKey] = attempts + 1
							t.attemptPeerReconnection(peer, resolvedEndpoints)
							break
						}
					}
				}
			} else {
				t.logger.Error("Peer reconnection failed after maximum retries",
					"tunnel", t.name,
					"peer", peerKey[:8]+"...",
					"attempts", attempts)
			}
		}
	}
}

// MARK: Parse timestamp safely
func parseTimestamp(timestampStr string) int64 {
	if timestamp, err := time.Parse("1136239445", timestampStr); err == nil {
		return timestamp.Unix()
	}
	// Try parsing as Unix timestamp
	if timestamp, err := time.Parse("1642782847", timestampStr); err == nil {
		return timestamp.Unix()
	}
	// Fallback: try direct conversion
	if timestamp, err := time.ParseDuration(timestampStr + "s"); err == nil {
		return int64(timestamp.Seconds())
	}
	return 0
}

// MARK: Check if peer needs reconnection
func (t *Tunnel) checkPeerReconnection(peerKey, currentEndpoint string, lastHandshakes map[string]time.Time, resolvedEndpoints map[string]string, reconnectionAttempts map[string]int) {
	// If we have a last known good handshake, check if it's recent
	if lastHandshake, exists := lastHandshakes[peerKey]; exists {
		if time.Since(lastHandshake) < 2*time.Minute {
			return // Recent handshake, connection is likely fine
		}
	}

	// Find the peer config for this public key
	for _, peer := range t.config.Peers {
		if peerPublicKeyHex, err := base64ToHex(peer.PublicKey); err == nil && peerPublicKeyHex == peerKey {
			// Check if endpoint resolution is needed
			if peer.Endpoint != "" && currentEndpoint != "" {
				// Try re-resolving the endpoint
				if newEndpoint, err := resolveEndpoint(peer.Endpoint); err == nil {
					if newEndpoint != currentEndpoint {
						t.logger.Info("Endpoint IP changed, updating peer",
							"tunnel", t.name,
							"peer", peer.Name,
							"old_endpoint", currentEndpoint,
							"new_endpoint", newEndpoint)

						reconnectionAttempts[peerKey]++
						t.attemptPeerReconnection(peer, resolvedEndpoints)
					}
				} else {
					t.logger.Error("Failed to re-resolve endpoint",
						"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", err)
				}
			}
			break
		}
	}
}

// MARK: Attempt peer reconnection
func (t *Tunnel) attemptPeerReconnection(peer config.PeerConfig, resolvedEndpoints map[string]string) {
	if peer.Endpoint == "" {
		return
	}

	// Resolve the endpoint
	newEndpoint, err := resolveEndpoint(peer.Endpoint)
	if err != nil {
		t.logger.Error("Failed to resolve endpoint for reconnection",
			"tunnel", t.name, "peer", peer.Name, "endpoint", peer.Endpoint, "error", err)
		return
	}

	// Check if endpoint actually changed
	peerKey := fmt.Sprintf("%s:%s", t.name, peer.Name)
	if lastResolved, exists := resolvedEndpoints[peerKey]; exists && lastResolved == newEndpoint {
		return // No change in resolved IP
	}

	resolvedEndpoints[peerKey] = newEndpoint

	// Update just this peer's endpoint via UAPI
	publicKeyHex, err := base64ToHex(peer.PublicKey)
	if err != nil {
		t.logger.Error("Invalid public key for peer reconnection",
			"tunnel", t.name, "peer", peer.Name, "error", err)
		return
	}

	uapi := fmt.Sprintf("public_key=%s\nendpoint=%s\n", publicKeyHex, newEndpoint)

	if err := t.device.IpcSetOperation(strings.NewReader(uapi)); err != nil {
		t.logger.Error("Failed to update peer endpoint",
			"tunnel", t.name, "peer", peer.Name, "endpoint", newEndpoint, "error", err)
	} else {
		t.logger.Info("Updated peer endpoint",
			"tunnel", t.name, "peer", peer.Name, "new_endpoint", newEndpoint)
	}
}

// MARK: Test connectivity to peer networks
func (t *Tunnel) TestConnectivity(ctx context.Context, target string) error {
	ip := net.ParseIP(target)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", target)
	}

	conn, err := net.DialTimeout("ip4:icmp", target, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connectivity test failed to %s: %w", target, err)
	}
	conn.Close()

	t.logger.Info("Connectivity test successful", "tunnel", t.name, "target", target)
	return nil
}

// MARK: Apply WireGuard configuration
func (t *Tunnel) applyConfig() error {
	if t.device == nil {
		return fmt.Errorf("device not initialized")
	}

	privateKey := strings.TrimSpace(t.config.PrivateKey)
	if privateKey == "" {
		return fmt.Errorf("private_key must be provided in tunnel config")
	}

	privateKeyHex, err := base64ToHex(privateKey)
	if err != nil {
		return fmt.Errorf("invalid private key format: %w", err)
	}

	uapi := fmt.Sprintf("private_key=%s\n", privateKeyHex)

	if t.config.ListenPort > 0 {
		uapi += fmt.Sprintf("listen_port=%d\n", t.config.ListenPort)
	}

	for _, peer := range t.config.Peers {
		publicKeyHex, err := base64ToHex(peer.PublicKey)
		if err != nil {
			return fmt.Errorf("invalid public key format for peer %s: %w", peer.Name, err)
		}

		uapi += fmt.Sprintf("public_key=%s\n", publicKeyHex)

		if peer.Endpoint != "" {
			// Resolve hostname if needed
			resolvedEndpoint, err := resolveEndpoint(peer.Endpoint)
			if err != nil {
				return fmt.Errorf("failed to resolve endpoint %s for peer %s: %w", peer.Endpoint, peer.Name, err)
			}
			uapi += fmt.Sprintf("endpoint=%s\n", resolvedEndpoint)
		}

		if peer.Preshared != "" {
			presharedHex, err := base64ToHex(peer.Preshared)
			if err != nil {
				return fmt.Errorf("invalid preshared key format for peer %s: %w", peer.Name, err)
			}
			uapi += fmt.Sprintf("preshared_key=%s\n", presharedHex)
		}

		for _, allowedIP := range peer.AllowedIPs {
			uapi += fmt.Sprintf("allowed_ip=%s\n", allowedIP)
		}

		if peer.Persistent || peer.PersistentKeepaliveInt > 0 {
			keepalive := peer.PersistentKeepaliveInt
			if keepalive == 0 {
				keepalive = 25
			}
			uapi += fmt.Sprintf("persistent_keepalive_interval=%d\n", keepalive)
		}
	}

	return t.device.IpcSetOperation(strings.NewReader(uapi))
}

// MARK: Resolve endpoint hostname to IP with caching
func resolveEndpoint(endpoint string) (string, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid endpoint format: %s", endpoint)
	}

	hostname := parts[0]
	port := parts[1]

	// Check if it's already an IP address
	if net.ParseIP(hostname) != nil {
		return endpoint, nil
	}

	// Resolve hostname with timeout
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: 10 * time.Second,
			}
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

	// Use the first IPv4 address, or first IPv6 if no IPv4
	var selectedIP net.IP
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			selectedIP = ip.IP
			break
		}
	}
	if selectedIP == nil {
		selectedIP = ips[0].IP // Use first IP (likely IPv6)
	}

	return fmt.Sprintf("%s:%s", selectedIP.String(), port), nil
}

// MARK: Test connectivity to peer endpoint
func (t *Tunnel) testEndpointConnectivity(endpoint string) bool {
	conn, err := net.DialTimeout("udp", endpoint, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// MARK: Convert base64 to hex
func base64ToHex(b64 string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("base64 decode error: %w", err)
	}

	if len(decoded) != 32 {
		return "", fmt.Errorf("key must be 32 bytes, got %d", len(decoded))
	}

	return fmt.Sprintf("%x", decoded), nil
}

type TUNWrapper struct {
	iface  *water.Interface
	mtu    int
	name   string
	events chan tun.Event
}

func (w *TUNWrapper) File() *os.File {
	return nil
}

func (w *TUNWrapper) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if len(bufs) == 0 {
		return 0, nil
	}

	n, err := w.iface.Read(bufs[0][offset:])
	if n > 0 {
		sizes[0] = n
		return 1, nil
	}
	return 0, err
}

func (w *TUNWrapper) Write(bufs [][]byte, offset int) (int, error) {
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

func (w *TUNWrapper) Flush() error {
	return nil
}

func (w *TUNWrapper) MTU() (int, error) {
	return w.mtu, nil
}

func (w *TUNWrapper) Name() (string, error) {
	return w.name, nil
}

func (w *TUNWrapper) Events() <-chan tun.Event {
	if w.events == nil {
		w.events = make(chan tun.Event, 1)
		w.events <- tun.EventUp
	}
	return w.events
}

func (w *TUNWrapper) Close() error {
	if w.events != nil {
		close(w.events)
	}
	return w.iface.Close()
}

func (w *TUNWrapper) BatchSize() int {
	return 1
}
