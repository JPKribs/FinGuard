package config

// MARK: setDefaults
// Applies default values to server, update, services, and WireGuard tunnel settings.
func (c *Config) setDefaults() {
	if c.Server.HTTPAddr == "" {
		c.Server.HTTPAddr = DefaultHTTPAddr
	}
	if c.Server.ProxyAddr == "" {
		c.Server.ProxyAddr = DefaultProxyAddr
	}
	if c.Server.WebRoot == "" {
		c.Server.WebRoot = DefaultWebRoot
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
