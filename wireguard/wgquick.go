package wireguard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

const (
	wgQuickConfigDir = "/tmp"
)

// MARK: NewWgQuickTunnel
func NewWgQuickTunnel(cfg config.TunnelConfig, paths config.WireGuardPaths, logger *internal.Logger, resolver *AsyncResolver) (*WgQuickTunnel, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("tunnel name cannot be empty")
	}

	if !isWgQuickAvailable(paths) {
		return nil, fmt.Errorf("wg-quick not available")
	}

	if logger == nil {
		logger = &internal.Logger{}
	}

	if resolver == nil {
		resolver = NewAsyncResolver()
	}

	configPath := filepath.Join(wgQuickConfigDir, cfg.Name+".conf")

	return &WgQuickTunnel{
		name:           cfg.Name,
		config:         cfg,
		paths:          paths,
		logger:         logger,
		resolver:       resolver,
		configPath:     configPath,
		stopMonitoring: make(chan struct{}),
		reconnectCount: make(map[string]int),
		endpointCache:  make(map[string]string),
	}, nil
}

// MARK: isWgQuickAvailable
func isWgQuickAvailable(paths config.WireGuardPaths) bool {
	if _, err := os.Stat("/sys/module/wireguard"); err != nil {
		return false
	}

	if paths.WgQuick != "" {
		if _, err := os.Stat(paths.WgQuick); err == nil {
			return paths.ValidatePaths() == nil
		}
	}

	if _, err := exec.LookPath("wg-quick"); err == nil {
		return paths.ValidatePaths() == nil
	}

	return false
}

// MARK: Start
func (wq *WgQuickTunnel) Start(ctx context.Context) error {
	wq.mu.Lock()
	defer wq.mu.Unlock()

	if atomic.LoadInt64(&wq.running) == 1 {
		return fmt.Errorf("tunnel %s already running", wq.name)
	}

	wq.logger.Info("Starting wg-quick tunnel", "name", wq.name)

	if err := wq.ensureConfigDirectory(); err != nil {
		return fmt.Errorf("ensuring config directory: %w", err)
	}

	if err := wq.generateConfig(); err != nil {
		return fmt.Errorf("generating config: %w", err)
	}

	if err := wq.startWithWgQuick(); err != nil {
		wq.cleanupOnFailure()
		return fmt.Errorf("starting with wg-quick: %w", err)
	}

	atomic.StoreInt64(&wq.running, 1)
	wq.logger.Info("WG-Quick tunnel started", "name", wq.name, "config", wq.configPath)

	wq.startMonitoring(ctx)
	return nil
}

// MARK: ensureConfigDirectory
func (wq *WgQuickTunnel) ensureConfigDirectory() error {
	if err := os.MkdirAll(wgQuickConfigDir, 0755); err != nil {
		return fmt.Errorf("creating wireguard config directory: %w", err)
	}
	return nil
}

// MARK: generateConfig
func (wq *WgQuickTunnel) generateConfig() error {
	config := wq.buildWgQuickConfig()

	if err := os.WriteFile(wq.configPath, []byte(config), 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	wq.logger.Info("Generated wg-quick config", "path", wq.configPath)
	return nil
}

// MARK: buildWgQuickConfig
func (wq *WgQuickTunnel) buildWgQuickConfig() string {
	var config strings.Builder

	config.WriteString("[Interface]\n")
	config.WriteString(fmt.Sprintf("PrivateKey = %s\n", wq.config.PrivateKey))

	if len(wq.config.Addresses) > 0 {
		config.WriteString(fmt.Sprintf("Address = %s\n", strings.Join(wq.config.Addresses, ", ")))
	}

	if wq.config.ListenPort > 0 {
		config.WriteString(fmt.Sprintf("ListenPort = %d\n", wq.config.ListenPort))
	}

	mtu := wq.config.MTU
	if mtu <= 0 {
		mtu = 1420
	}
	config.WriteString(fmt.Sprintf("MTU = %d\n", mtu))

	if len(wq.config.Routes) > 0 {
		config.WriteString("Table = auto\n")
		config.WriteString(fmt.Sprintf("PostUp = %s\n", wq.buildPostUpCommands()))
		config.WriteString(fmt.Sprintf("PreDown = %s\n", wq.buildPreDownCommands()))
	}

	for _, peer := range wq.config.Peers {
		config.WriteString("\n[Peer]\n")
		config.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))

		if peer.Preshared != "" {
			config.WriteString(fmt.Sprintf("PresharedKey = %s\n", peer.Preshared))
		}

		config.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(peer.AllowedIPs, ", ")))

		if peer.Endpoint != "" {
			if resolved, ok := wq.resolver.ResolveFast(peer.Endpoint); ok {
				config.WriteString(fmt.Sprintf("Endpoint = %s\n", resolved))
			} else {
				config.WriteString(fmt.Sprintf("Endpoint = %s\n", peer.Endpoint))
			}
		}

		if peer.Persistent || peer.PersistentKeepaliveInt > 0 {
			keepalive := peer.PersistentKeepaliveInt
			if keepalive <= 0 {
				keepalive = defaultKeepalive
			}
			config.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", keepalive))
		}
	}

	return config.String()
}

// MARK: buildPostUpCommands
func (wq *WgQuickTunnel) buildPostUpCommands() string {
	var commands []string

	for _, route := range wq.config.Routes {
		cmd := fmt.Sprintf("%s route add %s dev %s", wq.paths.IpTool, route, wq.name)
		commands = append(commands, cmd)
	}

	return strings.Join(commands, "; ")
}

// MARK: buildPreDownCommands
func (wq *WgQuickTunnel) buildPreDownCommands() string {
	var commands []string

	for _, route := range wq.config.Routes {
		cmd := fmt.Sprintf("%s route del %s dev %s 2>/dev/null || true", wq.paths.IpTool, route, wq.name)
		commands = append(commands, cmd)
	}

	return strings.Join(commands, "; ")
}

// MARK: startWithWgQuick
func (wq *WgQuickTunnel) startWithWgQuick() error {
	wgQuickPath := wq.paths.WgQuick
	if wgQuickPath == "" {
		return fmt.Errorf("wg-quick path not configured")
	}

	cmd := exec.Command(wgQuickPath, "up", wq.name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wg-quick up failed: %w (output: %s)", err, string(output))
	}

	wq.logger.Info("WG-Quick interface brought up", "name", wq.name, "output", string(output))
	return nil
}

// MARK: Stop
func (wq *WgQuickTunnel) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&wq.running, 1, 0) {
		return nil
	}

	wq.logger.Info("Stopping wg-quick tunnel", "name", wq.name)

	wq.stopMonitoringRoutine()

	wq.mu.Lock()
	defer wq.mu.Unlock()

	if err := wq.stopWithWgQuick(); err != nil {
		wq.logger.Error("Failed to stop with wg-quick", "name", wq.name, "error", err)
	}

	if err := wq.cleanupConfig(); err != nil {
		wq.logger.Error("Failed to cleanup config", "name", wq.name, "error", err)
	}

	wq.lastError = nil
	wq.logger.Info("WG-Quick tunnel stopped", "name", wq.name)
	return nil
}

// MARK: stopWithWgQuick
func (wq *WgQuickTunnel) stopWithWgQuick() error {
	wgQuickPath := wq.paths.WgQuick
	if wgQuickPath == "" {
		return fmt.Errorf("wg-quick path not configured")
	}

	cmd := exec.Command(wgQuickPath, "down", wq.name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if !strings.Contains(string(output), "is not a WireGuard interface") {
			return fmt.Errorf("wg-quick down failed: %w (output: %s)", err, string(output))
		}
	}

	wq.logger.Info("WG-Quick interface brought down", "name", wq.name)
	return nil
}

// MARK: cleanupConfig
func (wq *WgQuickTunnel) cleanupConfig() error {
	if err := os.Remove(wq.configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing config file: %w", err)
	}
	return nil
}

// MARK: cleanupOnFailure
func (wq *WgQuickTunnel) cleanupOnFailure() {
	wq.stopWithWgQuick()
	wq.cleanupConfig()
}

// MARK: Update
func (wq *WgQuickTunnel) Update(ctx context.Context, cfg config.TunnelConfig) error {
	if cfg.Name != wq.name {
		return fmt.Errorf("cannot change tunnel name from %s to %s", wq.name, cfg.Name)
	}

	wq.mu.Lock()
	defer wq.mu.Unlock()

	oldConfig := wq.config
	wq.config = cfg

	if atomic.LoadInt64(&wq.running) == 1 {
		if err := wq.generateConfig(); err != nil {
			wq.config = oldConfig
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&wq.lastError)), unsafe.Pointer(&err))
			return fmt.Errorf("generating updated config: %w", err)
		}

		if err := wq.restartInterface(); err != nil {
			wq.config = oldConfig
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&wq.lastError)), unsafe.Pointer(&err))
			return fmt.Errorf("restarting interface: %w", err)
		}

		wq.logger.Info("Applied configuration update", "name", wq.name)
	}

	return nil
}

// MARK: restartInterface
func (wq *WgQuickTunnel) restartInterface() error {
	if err := wq.stopWithWgQuick(); err != nil {
		wq.logger.Warn("Failed to stop interface during restart", "error", err)
	}

	time.Sleep(1 * time.Second)

	return wq.startWithWgQuick()
}

// / MARK: Status
func (wq *WgQuickTunnel) Status(ctx context.Context) TunnelStatus {
	state := "stopped"
	if atomic.LoadInt64(&wq.running) == 1 {
		state = "running"
	}

	status := TunnelStatus{
		Name:      wq.name,
		State:     state,
		Interface: wq.name,
		MTU:       wq.config.MTU,
		Peers:     len(wq.config.Peers),
	}

	wq.mu.RLock()
	if wq.lastError != nil {
		status.Error = wq.lastError.Error()
	}
	wq.mu.RUnlock()

	return status
}

// MARK: startMonitoring
func (wq *WgQuickTunnel) startMonitoring(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-wq.stopMonitoring:
				return
			case <-ticker.C:
				wq.monitorConnection()
			}
		}
	}()
}

// MARK: stopMonitoringRoutine
func (wq *WgQuickTunnel) stopMonitoringRoutine() {
	select {
	case wq.stopMonitoring <- struct{}{}:
	default:
	}
}

// MARK: monitorConnection
func (wq *WgQuickTunnel) monitorConnection() {
	if atomic.LoadInt64(&wq.running) != 1 {
		return
	}

	cmd := exec.Command(wq.paths.WgTool, "show", wq.name, "latest-handshakes")
	output, err := cmd.Output()
	if err != nil {
		wq.logger.Debug("Failed to get handshake info", "tunnel", wq.name, "error", err)
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	staleThreshold := time.Now().Add(-2 * time.Minute).Unix()
	hasStaleConnections := false

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		lastHandshake := parseHandshakeTime(parts[1])
		if lastHandshake > 0 && lastHandshake < staleThreshold {
			hasStaleConnections = true
			wq.logger.Warn("Stale connection detected", "tunnel", wq.name, "peer", parts[0])
		}
	}

	if hasStaleConnections {
		wq.logger.Info("Attempting to refresh stale connections", "tunnel", wq.name)
		wq.refreshEndpoints()
	}
}

// MARK: refreshEndpoints
func (wq *WgQuickTunnel) refreshEndpoints() {
	for _, peer := range wq.config.Peers {
		if peer.Endpoint == "" {
			continue
		}

		go func(p config.PeerConfig) {
			resultChan := wq.resolver.ResolveAsync(p.Endpoint, 10*time.Second)
			select {
			case result := <-resultChan:
				if result.err == nil {
					wq.updatePeerEndpoint(p, result.endpoint)
				}
			case <-time.After(10 * time.Second):
				wq.logger.Debug("Endpoint resolution timeout", "tunnel", wq.name, "peer", p.Name)
			}
		}(peer)
	}
}

// MARK: updatePeerEndpoint
func (wq *WgQuickTunnel) updatePeerEndpoint(peer config.PeerConfig, newEndpoint string) {
	peerKey := fmt.Sprintf("%s:%s", wq.name, peer.Name)

	if lastResolved, exists := wq.endpointCache[peerKey]; exists && lastResolved == newEndpoint {
		return
	}

	wq.endpointCache[peerKey] = newEndpoint

	cmd := exec.Command(wq.paths.WgTool, "set", wq.name, "peer", peer.PublicKey, "endpoint", newEndpoint)
	if err := cmd.Run(); err != nil {
		wq.logger.Error("Failed to update peer endpoint",
			"tunnel", wq.name, "peer", peer.Name, "endpoint", newEndpoint, "error", err)
	} else {
		wq.logger.Info("Updated peer endpoint",
			"tunnel", wq.name, "peer", peer.Name, "new_endpoint", newEndpoint)
	}
}

// MARK: parseHandshakeTime
func parseHandshakeTime(timeStr string) int64 {
	if timeStr == "0" {
		return 0
	}

	timestamp, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return 0
	}

	return timestamp.Unix()
}
