package wireguard

import (
	"context"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
	"github.com/songgao/water"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// MARK: Manager
type Manager struct {
	logger        *internal.Logger
	tunnels       map[string]*Tunnel
	resolver      *AsyncResolver
	mu            sync.RWMutex
	running       int64
	lastError     error
	retryAttempts int32
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// MARK: TunnelManager
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

// MARK: TunnelStatus
type TunnelStatus struct {
	Name      string   `json:"name"`
	State     string   `json:"state"`
	Interface string   `json:"interface"`
	MTU       int      `json:"mtu"`
	Peers     int      `json:"peers"`
	Routes    []string `json:"routes,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// MARK: TUNDevice
type TUNDevice struct {
	iface *water.Interface
	name  string
	mtu   int
}

// MARK: Tunnel
type Tunnel struct {
	name             string
	config           config.TunnelConfig
	device           *device.Device
	tunDev           *TUNDevice
	logger           *internal.Logger
	resolver         *AsyncResolver
	running          int64
	mu               sync.RWMutex
	stopMonitoring   chan struct{}
	monitoringActive int64
	lastError        error
	reconnectCount   map[string]int
	endpointCache    map[string]string
	bufferPool       *PacketBufferPool
}

// MARK: AsyncResolver
type AsyncResolver struct {
	cache       sync.Map
	workQueue   chan *resolveRequest
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	activeCount int64

	stats struct {
		hits     uint64
		misses   uint64
		errors   uint64
		timeouts uint64
	}
}

// MARK: cacheEntry
type cacheEntry struct {
	ip        net.IP
	port      string
	timestamp time.Time
	ttl       time.Duration
}

// MARK: resolveRequest
type resolveRequest struct {
	hostname string
	port     string
	result   chan *resolveResult
	timeout  time.Duration
}

// MARK: resolveResult
type resolveResult struct {
	endpoint  string
	err       error
	fromCache bool
}

// MARK: PacketBuffer
type PacketBuffer struct {
	data   []byte
	length int
	next   unsafe.Pointer
}

// MARK: PacketBufferPool
type PacketBufferPool struct {
	head unsafe.Pointer
	size int32
	max  int32
}

// MARK: TUNWrapper
type TUNWrapper struct {
	iface      *water.Interface
	mtu        int
	name       string
	events     chan tun.Event
	closed     int64
	bufferPool *PacketBufferPool
	batchSize  int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}
