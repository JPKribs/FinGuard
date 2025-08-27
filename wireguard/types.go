package wireguard

import (
	"context"
	"sync"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/songgao/water"
	"golang.zx2c4.com/wireguard/device"
)

// MARK: manager.go

type Manager struct {
	logger        *internal.Logger
	tunnels       map[string]*Tunnel
	mu            sync.RWMutex
	running       bool
	lastError     error
	retryAttempts int
}

type TunnelManager interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	CreateTunnel(ctx context.Context, cfg config.TunnelConfig) error
	UpdateTunnel(ctx context.Context, cfg config.TunnelConfig) error
	DeleteTunnel(ctx context.Context, name string) error
	Status(ctx context.Context, name string) (TunnelStatus, error)
	ListTunnels(ctx context.Context) ([]TunnelStatus, error)
	IsReady() bool
	Recover(ctx context.Context) error
}

type TunnelStatus struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Interface string `json:"interface"`
	MTU       int    `json:"mtu"`
	Peers     int    `json:"peers"`
	Error     string `json:"error,omitempty"`
}

// MARK: tuntap.go

type TUNDevice struct {
	iface *water.Interface
	name  string
	mtu   int
}

// MARK: wgdevice.go

type Tunnel struct {
	name             string
	config           config.TunnelConfig
	device           *device.Device
	tunDev           *TUNDevice
	logger           *internal.Logger
	running          bool
	mu               sync.RWMutex
	stopMonitoring   chan struct{}
	monitoringActive bool
	lastError        error
	reconnectCount   map[string]int
	endpointCache    map[string]string
}
