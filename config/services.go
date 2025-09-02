package config

import (
	"fmt"
	"net/url"
	"strings"
)

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
