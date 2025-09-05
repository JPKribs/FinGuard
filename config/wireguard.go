package config

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	WireGuardModeWgQuick   WireGuardMode = "wg-quick"
	WireGuardModeKernel    WireGuardMode = "kernel"
	WireGuardModeUserspace WireGuardMode = "userspace"
	WireGuardModeAuto      WireGuardMode = "auto"
)

// MARK: LoadWireGuard
func (c *Config) LoadWireGuard() error {
	return c.loadWireGuardFile()
}

// MARK: GetDefaultPaths
func GetDefaultPaths() WireGuardPaths {
	return WireGuardPaths{
		WgTool:    findExecutable("wg", []string{"/usr/bin/wg", "/usr/local/bin/wg", "/bin/wg"}),
		WgQuick:   findExecutable("wg-quick", []string{"/usr/bin/wg-quick", "/usr/local/bin/wg-quick", "/bin/wg-quick"}),
		IpTool:    findExecutable("ip", []string{"/sbin/ip", "/usr/sbin/ip", "/bin/ip", "/usr/bin/ip"}),
		ModProbe:  findExecutable("modprobe", []string{"/sbin/modprobe", "/usr/sbin/modprobe"}),
		SysCtl:    findExecutable("sysctl", []string{"/sbin/sysctl", "/usr/sbin/sysctl"}),
		SystemCtl: findExecutable("systemctl", []string{"/bin/systemctl", "/usr/bin/systemctl"}),
	}
}

// MARK: findExecutable
func findExecutable(name string, paths []string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return name
}

// MARK: ValidatePaths
func (p *WireGuardPaths) ValidatePaths() error {
	requiredTools := map[string]string{
		"wg tool":   p.WgTool,
		"wg-quick":  p.WgQuick,
		"ip tool":   p.IpTool,
		"modprobe":  p.ModProbe,
		"sysctl":    p.SysCtl,
		"systemctl": p.SystemCtl,
	}

	var missingTools []string
	for toolName, toolPath := range requiredTools {
		if toolPath == "" {
			missingTools = append(missingTools, toolName)
			continue
		}

		if !filepath.IsAbs(toolPath) {
			if _, err := exec.LookPath(toolPath); err != nil {
				missingTools = append(missingTools, fmt.Sprintf("%s (%s)", toolName, toolPath))
			}
		} else {
			if _, err := os.Stat(toolPath); err != nil {
				missingTools = append(missingTools, fmt.Sprintf("%s (%s)", toolName, toolPath))
			}
		}
	}

	if len(missingTools) > 0 {
		return fmt.Errorf("missing or invalid tools: %s", strings.Join(missingTools, ", "))
	}

	return nil
}

// MARK: SetDefaults
func (p *WireGuardPaths) SetDefaults() {
	defaults := GetDefaultPaths()

	if p.WgTool == "" {
		p.WgTool = defaults.WgTool
	}
	if p.WgQuick == "" {
		p.WgQuick = defaults.WgQuick
	}
	if p.IpTool == "" {
		p.IpTool = defaults.IpTool
	}
	if p.ModProbe == "" {
		p.ModProbe = defaults.ModProbe
	}
	if p.SysCtl == "" {
		p.SysCtl = defaults.SysCtl
	}
	if p.SystemCtl == "" {
		p.SystemCtl = defaults.SystemCtl
	}
}

// MARK: GetWireGuardMode
func (w *WireGuardConfig) GetWireGuardMode() WireGuardMode {
	switch w.Mode {
	case WireGuardModeWgQuick:
		if w.isWgQuickAvailable() {
			return WireGuardModeWgQuick
		}
		fallthrough
	case WireGuardModeKernel:
		if w.isKernelWireGuardAvailable() {
			return WireGuardModeKernel
		}
		return WireGuardModeUserspace
	case WireGuardModeAuto:
		if w.isWgQuickAvailable() {
			return WireGuardModeWgQuick
		}
		if w.isKernelWireGuardAvailable() {
			return WireGuardModeKernel
		}
		return WireGuardModeUserspace
	default:
		return WireGuardModeUserspace
	}
}

// MARK: isWgQuickAvailable
func (w *WireGuardConfig) isWgQuickAvailable() bool {
	if _, err := os.Stat("/sys/module/wireguard"); err != nil {
		return false
	}

	if w.Paths.WgQuick != "" {
		if _, err := os.Stat(w.Paths.WgQuick); err == nil {
			return w.Paths.ValidatePaths() == nil
		}
	}

	if _, err := exec.LookPath("wg-quick"); err == nil {
		return w.Paths.ValidatePaths() == nil
	}

	return false
}

// MARK: isKernelWireGuardAvailable
func (w *WireGuardConfig) isKernelWireGuardAvailable() bool {
	kernelModPath := "/sys/module/wireguard"
	if _, err := os.Stat(kernelModPath); err != nil {
		return false
	}

	procModulesPath := "/proc/modules"
	if data, err := os.ReadFile(procModulesPath); err == nil {
		if !strings.Contains(string(data), "wireguard") {
			return false
		}
	}

	return w.Paths.ValidatePaths() == nil
}

// MARK: SetWireGuardMode
func (c *Config) SetWireGuardMode(mode WireGuardMode) error {
	if mode == WireGuardModeKernel && !c.WireGuard.isKernelWireGuardAvailable() {
		return fmt.Errorf("kernel WireGuard module not available or tools missing")
	}

	c.WireGuard.Mode = mode
	return c.SaveWireGuard()
}

// MARK: LoadWireGuardWithDefaults
func (c *Config) LoadWireGuardWithDefaults() error {
	if err := c.LoadWireGuard(); err != nil {
		if os.IsNotExist(err) {
			c.WireGuard = WireGuardConfig{
				Mode:    WireGuardModeAuto,
				Paths:   GetDefaultPaths(),
				Tunnels: []TunnelConfig{},
			}
			return c.SaveWireGuard()
		}
		return err
	}

	if c.WireGuard.Mode == "" {
		c.WireGuard.Mode = WireGuardModeAuto
	}

	c.WireGuard.Paths.SetDefaults()

	return c.SaveWireGuard()
}

// MARK: UpdateToolPaths
func (c *Config) UpdateToolPaths(paths WireGuardPaths) error {
	if err := paths.ValidatePaths(); err != nil {
		return fmt.Errorf("invalid tool paths: %w", err)
	}

	c.WireGuard.Paths = paths
	return c.SaveWireGuard()
}

// MARK: GetToolPath
func (c *Config) GetToolPath(tool string) string {
	switch tool {
	case "wg":
		return c.WireGuard.Paths.WgTool
	case "wg-quick":
		return c.WireGuard.Paths.WgQuick
	case "ip":
		return c.WireGuard.Paths.IpTool
	case "modprobe":
		return c.WireGuard.Paths.ModProbe
	case "sysctl":
		return c.WireGuard.Paths.SysCtl
	case "systemctl":
		return c.WireGuard.Paths.SystemCtl
	default:
		return ""
	}
}

// MARK: EnsureKernelRequirements
func (c *Config) EnsureKernelRequirements() error {
	if !c.WireGuard.isKernelWireGuardAvailable() {
		return fmt.Errorf("kernel WireGuard module not loaded or tools missing")
	}

	return c.WireGuard.Paths.ValidatePaths()
}

// MARK: InstallKernelRequirements
func (c *Config) InstallKernelRequirements() error {
	commands := [][]string{
		{c.WireGuard.Paths.ModProbe, "wireguard"},
	}

	if _, err := exec.LookPath("apt-get"); err == nil {
		commands = append(commands, []string{"apt-get", "update"})
		commands = append(commands, []string{"apt-get", "install", "-y", "wireguard-tools", "iproute2"})
	} else if _, err := exec.LookPath("yum"); err == nil {
		commands = append(commands, []string{"yum", "install", "-y", "epel-release"})
		commands = append(commands, []string{"yum", "install", "-y", "wireguard-tools", "iproute"})
	} else if _, err := exec.LookPath("dnf"); err == nil {
		commands = append(commands, []string{"dnf", "install", "-y", "wireguard-tools", "iproute"})
	} else {
		return fmt.Errorf("unsupported package manager")
	}

	for _, cmd := range commands {
		if err := runCommand(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("failed to run %s: %w", strings.Join(cmd, " "), err)
		}
	}

	c.WireGuard.Paths.SetDefaults()
	return c.SaveWireGuard()
}

// MARK: runCommand
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// MARK: AddTunnel
// Adds a new WireGuard tunnel configuration and persists it.
func (c *Config) AddTunnel(tunnel TunnelConfig) error {
	if err := c.validateTunnelConfig(tunnel); err != nil {
		return err
	}

	for _, existing := range c.WireGuard.Tunnels {
		if strings.EqualFold(existing.Name, tunnel.Name) {
			return fmt.Errorf("tunnel %s already exists", tunnel.Name)
		}
	}

	c.WireGuard.Tunnels = append(c.WireGuard.Tunnels, tunnel)
	return c.SaveWireGuard()
}

// MARK: UpdateTunnel
// Updates an existing WireGuard tunnel configuration and persists it.
func (c *Config) UpdateTunnel(tunnel TunnelConfig) error {
	if err := c.validateTunnelConfig(tunnel); err != nil {
		return err
	}

	for i, existing := range c.WireGuard.Tunnels {
		if strings.EqualFold(existing.Name, tunnel.Name) {
			c.WireGuard.Tunnels[i] = tunnel
			return c.SaveWireGuard()
		}
	}

	return fmt.Errorf("tunnel %s not found", tunnel.Name)
}

// MARK: RemoveTunnel
// Removes a WireGuard tunnel configuration by name and persists it.
func (c *Config) RemoveTunnel(name string) error {
	for i, tunnel := range c.WireGuard.Tunnels {
		if strings.EqualFold(tunnel.Name, name) {
			c.WireGuard.Tunnels = append(c.WireGuard.Tunnels[:i], c.WireGuard.Tunnels[i+1:]...)
			return c.SaveWireGuard()
		}
	}
	return fmt.Errorf("tunnel %s not found", name)
}

// MARK: validateTunnelConfig
// Validates a single WireGuard tunnel configuration.
func (c *Config) validateTunnelConfig(tunnel TunnelConfig) error {
	if tunnel.Name == "" {
		return fmt.Errorf("tunnel name cannot be empty")
	}
	if tunnel.PrivateKey == "" {
		return fmt.Errorf("tunnel %s missing private_key", tunnel.Name)
	}
	if tunnel.ListenPort < 0 || tunnel.ListenPort > 65535 {
		return fmt.Errorf("tunnel %s has invalid listen_port: %d", tunnel.Name, tunnel.ListenPort)
	}

	for _, addr := range tunnel.Addresses {
		if _, _, err := net.ParseCIDR(addr); err != nil {
			return fmt.Errorf("invalid address %s in tunnel %s: %w", addr, tunnel.Name, err)
		}
	}

	for _, route := range tunnel.Routes {
		if _, _, err := net.ParseCIDR(route); err != nil {
			return fmt.Errorf("invalid route %s in tunnel %s: %w", route, tunnel.Name, err)
		}
	}

	for _, peer := range tunnel.Peers {
		if err := c.validatePeerConfig(peer, tunnel.Name); err != nil {
			return err
		}
	}

	return nil
}

// MARK: validatePeerConfig
// Validates a single peer configuration within a tunnel.
func (c *Config) validatePeerConfig(peer PeerConfig, tunnelName string) error {
	if peer.Name == "" {
		return fmt.Errorf("peer in tunnel %s missing name", tunnelName)
	}
	if peer.PublicKey == "" {
		return fmt.Errorf("peer %s in tunnel %s missing public_key", peer.Name, tunnelName)
	}

	for _, ip := range peer.AllowedIPs {
		if _, _, err := net.ParseCIDR(ip); err != nil {
			return fmt.Errorf("invalid allowed_ip %s in peer %s (tunnel %s): %w",
				ip, peer.Name, tunnelName, err)
		}
	}

	if peer.Endpoint != "" {
		if err := c.validateEndpoint(peer.Endpoint); err != nil {
			return fmt.Errorf("invalid endpoint %s in peer %s (tunnel %s): %w",
				peer.Endpoint, peer.Name, tunnelName, err)
		}
	}

	return nil
}

// MARK: validateEndpoint
// Validates a WireGuard peer endpoint address.
func (c *Config) validateEndpoint(endpoint string) error {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return fmt.Errorf("endpoint must be in format 'host:port'")
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port number")
	}

	return nil
}
