package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultHTTPAddr        = "0.0.0.0:8080"
	DefaultProxyAddr       = "0.0.0.0:80"
	DefaultLogLevel        = "info"
	DefaultMTU             = 1420
	DefaultKeepalive       = 25
	DefaultMonitorInterval = 30
	DefaultStaleTimeout    = 300
	DefaultRetries         = 3
	ServicesFileName       = "services.yaml"
	WireGuardFileName      = "wireguard.yaml"
	UpdateFileName         = "update.yaml"
	DefaultUpdateSchedule  = "0 3 * * *"
)

// MARK: Load
// Loads configuration from YAML file, applies defaults, validates, and loads external configs.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.setDefaults()

	if err := cfg.loadExternalConfigs(); err != nil {
		return nil, fmt.Errorf("loading external configs: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// MARK: setDefaults
// Applies default values to server, update, services, and WireGuard tunnel settings.
func (c *Config) setDefaults() {
	if c.Server.HTTPAddr == "" {
		c.Server.HTTPAddr = DefaultHTTPAddr
	}
	if c.Server.ProxyAddr == "" {
		c.Server.ProxyAddr = DefaultProxyAddr
	}
	if c.Log.Level == "" {
		c.Log.Level = DefaultLogLevel
	}
	if c.ServicesFile == "" {
		c.ServicesFile = ServicesFileName
	}
	if c.WireGuardFile == "" {
		c.WireGuardFile = WireGuardFileName
	}
	if c.UpdateFile == "" {
		c.UpdateFile = UpdateFileName
	}

	if c.Update.Schedule == "" {
		c.Update.Schedule = DefaultUpdateSchedule
	}
	if c.Update.BackupDir == "" {
		c.Update.BackupDir = "./backups"
	}

	for i := range c.WireGuard.Tunnels {
		tunnel := &c.WireGuard.Tunnels[i]
		if tunnel.MTU == 0 {
			tunnel.MTU = DefaultMTU
		}
		if tunnel.MonitorInterval == 0 {
			tunnel.MonitorInterval = DefaultMonitorInterval
		}
		if tunnel.StaleConnectionTimeout == 0 {
			tunnel.StaleConnectionTimeout = DefaultStaleTimeout
		}
		if tunnel.ReconnectionRetries == 0 {
			tunnel.ReconnectionRetries = DefaultRetries
		}

		for j := range tunnel.Peers {
			peer := &tunnel.Peers[j]
			if peer.PersistentKeepaliveInt > 0 {
				peer.Persistent = true
			} else if peer.Persistent && peer.PersistentKeepaliveInt == 0 {
				peer.PersistentKeepaliveInt = DefaultKeepalive
			}
		}
	}
}

// MARK: loadExternalConfigs
// Loads services, WireGuard, and update configuration files from disk.
func (c *Config) loadExternalConfigs() error {
	if err := c.loadServicesFile(); err != nil {
		return fmt.Errorf("loading services file: %w", err)
	}
	if err := c.loadWireGuardFile(); err != nil {
		return fmt.Errorf("loading wireguard file: %w", err)
	}
	if err := c.loadUpdateFile(); err != nil {
		return fmt.Errorf("loading update file: %w", err)
	}
	return nil
}

// MARK: loadServicesFile
// Loads or creates the services.yaml configuration file.
func (c *Config) loadServicesFile() error {
	if _, err := os.Stat(c.ServicesFile); os.IsNotExist(err) {
		return c.createEmptyFile(c.ServicesFile, struct {
			Services []ServiceConfig `yaml:"services"`
		}{})
	}

	data, err := os.ReadFile(c.ServicesFile)
	if err != nil {
		return fmt.Errorf("reading services file: %w", err)
	}

	var servicesConfig struct {
		Services []ServiceConfig `yaml:"services"`
	}

	if err := yaml.Unmarshal(data, &servicesConfig); err != nil {
		return fmt.Errorf("parsing services file: %w", err)
	}

	c.Services = servicesConfig.Services
	return nil
}

// MARK: loadWireGuardFile
// Loads or creates the wireguard.yaml configuration file.
func (c *Config) loadWireGuardFile() error {
	if _, err := os.Stat(c.WireGuardFile); os.IsNotExist(err) {
		return c.createEmptyFile(c.WireGuardFile, WireGuardConfig{})
	}

	data, err := os.ReadFile(c.WireGuardFile)
	if err != nil {
		return fmt.Errorf("reading wireguard file: %w", err)
	}

	if err := yaml.Unmarshal(data, &c.WireGuard); err != nil {
		return fmt.Errorf("parsing wireguard file: %w", err)
	}

	return nil
}

// MARK: loadUpdateFile
// Loads or creates the update.yaml configuration file.
func (c *Config) loadUpdateFile() error {
	if _, err := os.Stat(c.UpdateFile); os.IsNotExist(err) {
		return c.createEmptyFile(c.UpdateFile, UpdateConfig{
			Enabled:   false,
			Schedule:  DefaultUpdateSchedule,
			AutoApply: false,
			BackupDir: "./backups",
		})
	}

	data, err := os.ReadFile(c.UpdateFile)
	if err != nil {
		return fmt.Errorf("reading update file: %w", err)
	}

	if err := yaml.Unmarshal(data, &c.Update); err != nil {
		return fmt.Errorf("parsing update file: %w", err)
	}

	return nil
}

// MARK: createEmptyFile
// Helper to create an empty YAML file from a struct.
func (c *Config) createEmptyFile(filename string, structure interface{}) error {
	data, err := yaml.Marshal(structure)
	if err != nil {
		return fmt.Errorf("marshaling empty config: %w", err)
	}
	return os.WriteFile(filename, data, 0644)
}

// MARK: SaveServices
// Persists current service configurations to external file.
func (c *Config) SaveServices() error {
	return c.saveToFile(c.ServicesFile, struct {
		Services []ServiceConfig `yaml:"services"`
	}{Services: c.Services})
}

// MARK: SaveWireGuard
// Persists the current WireGuard configuration to file.
func (c *Config) SaveWireGuard() error {
	return c.saveToFile(c.WireGuardFile, c.WireGuard)
}

// MARK: saveToFile
// Generic helper for marshaling any struct to a YAML file.
func (c *Config) saveToFile(filename string, data interface{}) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(filename, yamlData, 0644)
}

// MARK: AddService
// Adds a new service configuration and saves to file.
func (c *Config) AddService(svc ServiceConfig) error {
	if err := c.validateServiceConfig(svc); err != nil {
		return err
	}

	for _, existing := range c.Services {
		if strings.EqualFold(existing.Name, svc.Name) {
			return fmt.Errorf("service %s already exists", svc.Name)
		}
	}

	c.Services = append(c.Services, svc)
	return c.SaveServices()
}

// MARK: RemoveService
// Removes a service configuration by name and saves to file.
func (c *Config) RemoveService(name string) error {
	for i, svc := range c.Services {
		if strings.EqualFold(svc.Name, name) {
			c.Services = append(c.Services[:i], c.Services[i+1:]...)
			return c.SaveServices()
		}
	}
	return fmt.Errorf("service %s not found", name)
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

	// Find and update the tunnel
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

// MARK: validate
// Validates the entire configuration including server, tunnels, and services.
func (c *Config) validate() error {
	if c.Server.AdminToken == "" || c.Server.AdminToken == "REPLACE_ME" {
		return fmt.Errorf("admin_token must be set to a secure value")
	}

	tunnelNames := make(map[string]bool, len(c.WireGuard.Tunnels))
	for _, tunnel := range c.WireGuard.Tunnels {
		if err := c.validateTunnelConfig(tunnel); err != nil {
			return err
		}
		tunnelNames[strings.ToLower(tunnel.Name)] = true
	}

	for _, service := range c.Services {
		if err := c.validateServiceConfig(service); err != nil {
			return err
		}
		if service.Tunnel != "" && !tunnelNames[strings.ToLower(service.Tunnel)] {
			return fmt.Errorf("service %s references unknown tunnel: %s", service.Name, service.Tunnel)
		}
	}

	return nil
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

// MARK: validateServiceConfig
// Validates a single service configuration.
func (c *Config) validateServiceConfig(svc ServiceConfig) error {
	if svc.Name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if svc.Upstream == "" {
		return fmt.Errorf("service %s missing upstream URL", svc.Name)
	}

	if _, err := url.Parse(svc.Upstream); err != nil {
		return fmt.Errorf("invalid upstream URL %s for service %s: %w", svc.Upstream, svc.Name, err)
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

// MARK: GetTunnel
// Returns a WireGuard tunnel configuration by name.
func (c *Config) GetTunnel(name string) *TunnelConfig {
	for i := range c.WireGuard.Tunnels {
		if strings.EqualFold(c.WireGuard.Tunnels[i].Name, name) {
			return &c.WireGuard.Tunnels[i]
		}
	}
	return nil
}

// MARK: GetServicesByTunnel
// Returns all services associated with a given tunnel.
func (c *Config) GetServicesByTunnel(tunnelName string) []ServiceConfig {
	services := make([]ServiceConfig, 0)
	for _, svc := range c.Services {
		if strings.EqualFold(svc.Tunnel, tunnelName) {
			services = append(services, svc)
		}
	}
	return services
}

// MARK: GetPortFromAddr
// Extracts the port number from an address string, defaulting to 80.
func GetPortFromAddr(addr string) int {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return 80
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return 80
	}

	return port
}

// MARK: SaveUpdate
// Persists the update configuration to file.
func (c *Config) SaveUpdate() error {
	return c.saveToFile(c.UpdateFile, c.Update)
}

// MARK: UpdateUpdateConfig
// Updates the update configuration and validates basic cron format.
func (c *Config) UpdateUpdateConfig(cfg UpdateConfig) error {
	if cfg.Schedule == "" {
		return fmt.Errorf("schedule cannot be empty")
	}

	fields := strings.Fields(cfg.Schedule)
	if len(fields) != 5 {
		return fmt.Errorf("invalid cron format, expected 5 fields")
	}

	c.Update = cfg
	return c.SaveUpdate()
}
