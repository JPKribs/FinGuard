package config

import (
	"fmt"
	"strings"
)

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
