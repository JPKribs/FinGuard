package main

import (
	"errors"
	"fmt"

	"github.com/JPKribs/FinGuard/config"
)

// MARK: addServices
// Adds all configured services to the proxy with error collection
func (app *Application) addServices() error {
	var errs []error
	addedServices := make(map[string]bool)

	for _, serviceCfg := range app.config.Services {
		if addedServices[serviceCfg.Name] {
			app.logger.Warn("Skipping duplicate service", "name", serviceCfg.Name)
			continue
		}

		if err := app.proxyServer.AddService(serviceCfg); err != nil {
			errs = append(errs, fmt.Errorf("service %s: %w", serviceCfg.Name, err))
			app.logger.Error("Failed to add service", "name", serviceCfg.Name, "error", err)
		} else {
			addedServices[serviceCfg.Name] = true
			app.logger.Info("Added service", "name", serviceCfg.Name, "upstream", serviceCfg.Upstream)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// MARK: publishServices
// Publishes configured services via mDNS
func (app *Application) publishServices() {
	if !app.config.Discovery.Enable || !app.config.Discovery.MDNS.Enabled {
		return
	}

	proxyPort := config.GetPortFromAddr(app.config.Server.ProxyAddr)

	for _, serviceCfg := range app.config.Services {
		if serviceCfg.PublishMDNS {
			if err := app.discoveryManager.PublishService(serviceCfg, proxyPort); err != nil {
				app.logger.Error("Failed to publish service via mDNS",
					"name", serviceCfg.Name, "error", err)
			} else {
				app.logger.Info("Published service via mDNS", "name", serviceCfg.Name)
			}
		}
	}
}

// MARK: updateReadiness
// Updates application readiness status based on component states
func (app *Application) updateReadiness() {
	isReady := app.tunnelManager.IsReady() && app.proxyServer.IsReady()

	if app.config.Discovery.Enable && app.config.Discovery.MDNS.Enabled {
		isReady = isReady && app.discoveryManager.IsReady()
	}

	app.healthCheck.SetReady(isReady)
}
