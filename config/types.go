package config

// MARK: Config
type Config struct {
	Server        ServerConfig    `yaml:"server"`
	WireGuard     WireGuardConfig `yaml:"wireguard"`
	Services      []ServiceConfig `yaml:"services"`
	Discovery     DiscoveryConfig `yaml:"discovery"`
	Log           LogConfig       `yaml:"log"`
	Update        UpdateConfig    `yaml:"update"`
	ServicesFile  string          `yaml:"services_file"`
	WireGuardFile string          `yaml:"wireguard_file"`
	UpdateFile    string          `yaml:"update_file"`
}

// MARK: UpdateConfig
type UpdateConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Schedule  string `yaml:"schedule"`
	AutoApply bool   `yaml:"auto_apply"`
	BackupDir string `yaml:"backup_dir"`
}

// MARK: ServerConfig
type ServerConfig struct {
	HTTPAddr   string `yaml:"http_addr"`
	ProxyAddr  string `yaml:"proxy_addr"`
	AdminToken string `yaml:"admin_token"`
	WebRoot    string `yaml:"web_root"`
}

// MARK: LogConfig
type LogConfig struct {
	Level string `yaml:"level"`
}

// MARK: WireGuardConfig
type WireGuardConfig struct {
	Tunnels []TunnelConfig `yaml:"tunnels"`
}

// MARK: TunnelConfig
type TunnelConfig struct {
	Name                   string       `yaml:"name"`
	ListenPort             int          `yaml:"listen_port"`
	PrivateKey             string       `yaml:"private_key"`
	MTU                    int          `yaml:"mtu"`
	Addresses              []string     `yaml:"addresses"`
	Routes                 []string     `yaml:"routes"`
	Peers                  []PeerConfig `yaml:"peers"`
	MonitorInterval        int          `yaml:"monitor_interval"`
	StaleConnectionTimeout int          `yaml:"stale_connection_timeout"`
	ReconnectionRetries    int          `yaml:"reconnection_retries"`
}

// MARK: PeerConfig
type PeerConfig struct {
	Name                   string   `yaml:"name"`
	PublicKey              string   `yaml:"public_key"`
	AllowedIPs             []string `yaml:"allowed_ips"`
	Endpoint               string   `yaml:"endpoint"`
	Preshared              string   `yaml:"preshared_key"`
	Persistent             bool     `yaml:"persistent_keepalive"`
	PersistentKeepaliveInt int      `yaml:"persistent_keepalive_interval"`
}

// MARK: ServiceConfig
type ServiceConfig struct {
	Name        string `yaml:"name" json:"name"`
	Upstream    string `yaml:"upstream" json:"upstream"`
	Jellyfin    bool   `yaml:"jellyfin" json:"jellyfin"`
	Websocket   bool   `yaml:"websocket" json:"websocket"`
	PublishMDNS bool   `yaml:"publish_mdns" json:"publish_mdns"`
	Default     bool   `yaml:"default" json:"default"`
	Tunnel      string `yaml:"tunnel" json:"tunnel"`
}

// MARK: DiscoveryConfig
type DiscoveryConfig struct {
	Enable bool       `yaml:"enable"`
	MDNS   MDNSConfig `yaml:"mdns"`
}

// MARK: MDNSConfig
type MDNSConfig struct {
	Enabled bool `yaml:"enabled"`
}
