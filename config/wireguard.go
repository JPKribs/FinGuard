package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

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
