package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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

// MARK: saveToFile
// Generic helper for marshaling any struct to a YAML file.
func (c *Config) saveToFile(filename string, data interface{}) error {
	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(filename, yamlData, 0644)
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

// MARK: SaveUpdate
// Persists the update configuration to file.
func (c *Config) SaveUpdate() error {
	return c.saveToFile(c.UpdateFile, c.Update)
}
