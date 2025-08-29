package v1

import (
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/discovery"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/JPKribs/FinGuard/proxy"
	"github.com/JPKribs/FinGuard/updater"
	"github.com/JPKribs/FinGuard/wireguard"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type ServiceStatusResponse struct {
	Name        string `json:"name"`
	Upstream    string `json:"upstream"`
	Status      string `json:"status"`
	Tunnel      string `json:"tunnel,omitempty"`
	Websocket   bool   `json:"websocket"`
	Default     bool   `json:"default"`
	PublishMDNS bool   `json:"publish_mdns"`
}

type TunnelStatus = wireguard.TunnelStatus

// MARK: TunnelCreateRequest
type TunnelCreateRequest struct {
	Name                   string              `json:"name"`
	ListenPort             int                 `json:"listen_port"`
	PrivateKey             string              `json:"private_key"`
	MTU                    int                 `json:"mtu"`
	Addresses              []string            `json:"addresses"`
	Routes                 []string            `json:"routes"`
	Peers                  []PeerCreateRequest `json:"peers"`
	MonitorInterval        int                 `json:"monitor_interval"`
	StaleConnectionTimeout int                 `json:"stale_connection_timeout"`
	ReconnectionRetries    int                 `json:"reconnection_retries"`
}

// MARK: PeerCreateRequest
type PeerCreateRequest struct {
	Name                string   `json:"name"`
	PublicKey           string   `json:"public_key"`
	AllowedIPs          []string `json:"allowed_ips"`
	Endpoint            string   `json:"endpoint"`
	PresharedKey        string   `json:"preshared_key"`
	PersistentKeepalive int      `json:"persistent_keepalive"`
}

type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

type LogResponse struct {
	Logs   []LogEntry `json:"logs"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

type APIServer struct {
	cfg              *config.Config
	proxyServer      *proxy.Server
	tunnelManager    wireguard.TunnelManager
	discoveryManager *discovery.Discovery
	logger           *internal.Logger
	updateManager    *updater.UpdateManager
}

type UpdateInfoResponse struct {
	Available         bool      `json:"available"`
	CurrentVersion    string    `json:"current_version"`
	LatestVersion     string    `json:"latest_version"`
	ReleaseNotes      string    `json:"release_notes"`
	LastCheckTime     time.Time `json:"last_check_time"`
	NextCheckTime     time.Time `json:"next_check_time"`
	UpdateSchedule    string    `json:"update_schedule"`
	AutoUpdateEnabled bool      `json:"auto_update_enabled"`
}

type UpdateConfigRequest struct {
	Enabled   bool   `json:"enabled"`
	Schedule  string `json:"schedule"`
	AutoApply bool   `json:"auto_apply"`
	BackupDir string `json:"backup_dir"`
}
