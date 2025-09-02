package config

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
