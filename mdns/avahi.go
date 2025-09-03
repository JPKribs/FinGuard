package mdns

import (
	"fmt"
	"runtime"

	"github.com/JPKribs/FinGuard/config"
	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
)

// MARK: tryAvahi
// Attempts to initialize Avahi; returns true if successful.
func (d *Discovery) tryAvahi() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	conn, err := dbus.SystemBus()
	if err != nil {
		d.logger.Debug("D-Bus not available", "error", err)
		return false
	}

	server, err := avahi.ServerNew(conn)
	if err != nil {
		d.logger.Debug("Avahi server creation failed", "error", err)
		conn.Close()
		return false
	}

	d.conn = conn
	d.server = server
	return true
}

// MARK: publishServiceAvahi
// Publishes service using Avahi backend.
func (d *Discovery) publishServiceAvahi(serviceName string, svc config.ServiceConfig, proxyPort int) error {
	txtRecords := d.buildTXTRecords(svc)

	if existingEntryGroup, exists := d.entryGroups[serviceName]; exists {
		if err := existingEntryGroup.Reset(); err != nil {
			d.logger.Error("Failed to reset existing entry group", "name", serviceName, "error", err)
		}
		delete(d.entryGroups, serviceName)
	}

	entryGroup, err := d.server.EntryGroupNew()
	if err != nil {
		return fmt.Errorf("failed to create entry group for service %s: %w", serviceName, err)
	}

	err = entryGroup.AddService(
		avahi.InterfaceUnspec,
		avahi.ProtoUnspec,
		0,
		serviceName+".finguard",
		"_http._tcp",
		"local",
		d.hostName,
		uint16(proxyPort),
		d.convertTXTRecords(txtRecords),
	)
	if err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to add service %s: %w", serviceName, err)
	}

	if err := entryGroup.Commit(); err != nil {
		entryGroup.Reset()
		return fmt.Errorf("failed to commit service %s: %w", serviceName, err)
	}

	d.entryGroups[serviceName] = entryGroup
	d.logger.Info("Published mDNS service via Avahi", "name", serviceName, "port", proxyPort, "host", d.hostName, "txt_records", len(txtRecords))
	return nil
}

// MARK: buildTXTRecords
// Creates TXT records for Avahi service advertisement.
func (d *Discovery) buildTXTRecords(svc config.ServiceConfig) []string {
	records := []string{
		fmt.Sprintf("service=%s", svc.Name),
		fmt.Sprintf("upstream=%s", svc.Upstream),
		"path=/",
	}
	if svc.Websocket {
		records = append(records, "websocket=true")
	}
	if svc.Default {
		records = append(records, "default=true")
	}
	if svc.Tunnel != "" {
		records = append(records, fmt.Sprintf("tunnel=%s", svc.Tunnel))
	}
	return records
}

// MARK: convertTXTRecords
// Converts string slice to byte slice slice for Avahi API.
func (d *Discovery) convertTXTRecords(records []string) [][]byte {
	result := make([][]byte, len(records))
	for i, record := range records {
		result[i] = []byte(record)
	}
	return result
}
