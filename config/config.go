package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
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
// Returns a services assigned to a specified tunnel.
func (c *Config) GetServicesByTunnel(tunnelName string) []ServiceConfig {
	services := make([]ServiceConfig, 0)
	for _, svc := range c.Services {
		if strings.EqualFold(svc.Tunnel, tunnelName) {
			services = append(services, svc)
		}
	}
	return services
}
