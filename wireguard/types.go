package wireguard

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
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
	Name      string `json:"name"`
	State     string `json:"state"`
	Interface string `json:"interface"`
	MTU       int    `json:"mtu"`
	Peers     int    `json:"peers"`
	Error     string `json:"error,omitempty"`
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

// MARK: OptimizedTUNWrapper
type OptimizedTUNWrapper struct {
	iface      *water.Interface
	mtu        int
	name       string
	events     chan tun.Event
	closed     int64
	bufferPool *PacketBufferPool
	batchSize  int

	readChan  chan *PacketBuffer
	writeChan chan []*PacketBuffer
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// MARK: NewAsyncResolver
func NewAsyncResolver() *AsyncResolver {
	ctx, cancel := context.WithCancel(context.Background())

	r := &AsyncResolver{
		workQueue: make(chan *resolveRequest, 256),
		ctx:       ctx,
		cancel:    cancel,
	}

	for i := 0; i < 4; i++ {
		r.wg.Add(1)
		go r.worker()
	}

	r.wg.Add(1)
	go r.cacheCleanup()

	return r
}

// MARK: ResolveAsync
func (r *AsyncResolver) ResolveAsync(endpoint string, timeout time.Duration) <-chan *resolveResult {
	result := make(chan *resolveResult, 1)

	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		result <- &resolveResult{err: fmt.Errorf("invalid endpoint format: %w", err)}
		return result
	}

	if ip := net.ParseIP(host); ip != nil {
		result <- &resolveResult{endpoint: endpoint}
		return result
	}

	if cached := r.getCached(host, port); cached != nil {
		atomic.AddUint64(&r.stats.hits, 1)
		result <- &resolveResult{
			endpoint:  fmt.Sprintf("%s:%s", cached.ip.String(), cached.port),
			fromCache: true,
		}
		return result
	}

	atomic.AddUint64(&r.stats.misses, 1)

	if atomic.LoadInt64(&r.activeCount) >= 32 {
		result <- &resolveResult{err: fmt.Errorf("resolver queue full")}
		return result
	}

	req := &resolveRequest{
		hostname: host,
		port:     port,
		result:   result,
		timeout:  timeout,
	}

	select {
	case r.workQueue <- req:
	default:
		result <- &resolveResult{err: fmt.Errorf("resolver busy")}
	}

	return result
}

// MARK: ResolveFast
func (r *AsyncResolver) ResolveFast(endpoint string) (string, bool) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return endpoint, false
	}

	if ip := net.ParseIP(host); ip != nil {
		return endpoint, true
	}

	if cached := r.getCached(host, port); cached != nil {
		atomic.AddUint64(&r.stats.hits, 1)
		return fmt.Sprintf("%s:%s", cached.ip.String(), cached.port), true
	}

	return endpoint, false
}

// MARK: getCached
func (r *AsyncResolver) getCached(hostname, port string) *cacheEntry {
	key := hostname + ":" + port
	if val, ok := r.cache.Load(key); ok {
		entry := val.(*cacheEntry)
		if time.Since(entry.timestamp) < entry.ttl {
			return entry
		}
		r.cache.Delete(key)
	}
	return nil
}

// MARK: setCached
func (r *AsyncResolver) setCached(hostname, port string, ip net.IP, ttl time.Duration) {
	key := hostname + ":" + port
	entry := &cacheEntry{
		ip:        ip,
		port:      port,
		timestamp: time.Now(),
		ttl:       ttl,
	}
	r.cache.Store(key, entry)
}

// MARK: worker
func (r *AsyncResolver) worker() {
	defer r.wg.Done()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, address)
		},
	}

	for {
		select {
		case <-r.ctx.Done():
			return
		case req := <-r.workQueue:
			r.processRequest(resolver, req)
		}
	}
}

// MARK: processRequest
func (r *AsyncResolver) processRequest(resolver *net.Resolver, req *resolveRequest) {
	atomic.AddInt64(&r.activeCount, 1)
	defer atomic.AddInt64(&r.activeCount, -1)

	timeout := req.timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.ctx, timeout)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctx, req.hostname)
	if err != nil {
		atomic.AddUint64(&r.stats.errors, 1)
		if ctx.Err() == context.DeadlineExceeded {
			atomic.AddUint64(&r.stats.timeouts, 1)
		}

		select {
		case req.result <- &resolveResult{err: err}:
		default:
		}
		return
	}

	if len(ips) == 0 {
		atomic.AddUint64(&r.stats.errors, 1)
		select {
		case req.result <- &resolveResult{err: fmt.Errorf("no IP addresses found for %s", req.hostname)}:
		default:
		}
		return
	}

	var selectedIP net.IP
	for _, ip := range ips {
		if ip.IP.To4() != nil {
			selectedIP = ip.IP
			break
		}
	}

	if selectedIP == nil {
		selectedIP = ips[0].IP
	}

	r.setCached(req.hostname, req.port, selectedIP, 300*time.Second)

	endpoint := fmt.Sprintf("%s:%s", selectedIP.String(), req.port)
	select {
	case req.result <- &resolveResult{endpoint: endpoint}:
	default:
	}
}

// MARK: cacheCleanup
func (r *AsyncResolver) cacheCleanup() {
	defer r.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.cleanExpiredEntries()
		}
	}
}

// MARK: cleanExpiredEntries
func (r *AsyncResolver) cleanExpiredEntries() {
	now := time.Now()
	toDelete := make([]string, 0, 64)

	r.cache.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if now.Sub(entry.timestamp) >= entry.ttl {
			toDelete = append(toDelete, key.(string))
		}
		return len(toDelete) < 1000
	})

	for _, key := range toDelete {
		r.cache.Delete(key)
	}
}

// MARK: Close
func (r *AsyncResolver) Close() {
	r.cancel()
	r.wg.Wait()
}

// MARK: GetStats
func (r *AsyncResolver) GetStats() (hits, misses, errors, timeouts uint64) {
	return atomic.LoadUint64(&r.stats.hits),
		atomic.LoadUint64(&r.stats.misses),
		atomic.LoadUint64(&r.stats.errors),
		atomic.LoadUint64(&r.stats.timeouts)
}

// MARK: NewPacketBufferPool
func NewPacketBufferPool(maxSize int) *PacketBufferPool {
	return &PacketBufferPool{
		max: int32(maxSize),
	}
}

// MARK: Get (PacketBufferPool)
func (p *PacketBufferPool) Get() *PacketBuffer {
	for {
		head := atomic.LoadPointer(&p.head)
		if head == nil {
			return &PacketBuffer{
				data: make([]byte, 2048),
			}
		}

		buf := (*PacketBuffer)(head)
		next := atomic.LoadPointer(&buf.next)

		if atomic.CompareAndSwapPointer(&p.head, head, next) {
			atomic.AddInt32(&p.size, -1)
			buf.next = nil
			buf.length = 0
			return buf
		}
	}
}

// MARK: Put (PacketBufferPool)
func (p *PacketBufferPool) Put(buf *PacketBuffer) {
	if buf == nil {
		return
	}

	if atomic.LoadInt32(&p.size) >= p.max {
		return
	}

	for {
		head := atomic.LoadPointer(&p.head)
		buf.next = head

		if atomic.CompareAndSwapPointer(&p.head, head, unsafe.Pointer(buf)) {
			atomic.AddInt32(&p.size, 1)
			return
		}
	}
}

// MARK: File (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) File() *os.File {
	return nil
}

// MARK: Read (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	if atomic.LoadInt64(&w.closed) != 0 || len(bufs) == 0 {
		return 0, net.ErrClosed
	}

	n, err := w.iface.Read(bufs[0][offset:])
	if n > 0 {
		sizes[0] = n
		return 1, nil
	}

	return 0, err
}

// MARK: Write (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Write(bufs [][]byte, offset int) (int, error) {
	if atomic.LoadInt64(&w.closed) != 0 {
		return 0, net.ErrClosed
	}

	count := 0
	for _, buf := range bufs {
		if len(buf) <= offset {
			continue
		}

		_, err := w.iface.Write(buf[offset:])
		if err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// MARK: Flush (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Flush() error {
	return nil
}

// MARK: MTU (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) MTU() (int, error) {
	if w.mtu <= 0 {
		return 1420, nil
	}
	return w.mtu, nil
}

// MARK: Name (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Name() (string, error) {
	return w.name, nil
}

// MARK: Events (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Events() <-chan tun.Event {
	return w.events
}

// MARK: Close (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) Close() error {
	if !atomic.CompareAndSwapInt64(&w.closed, 0, 1) {
		return nil
	}

	if w.cancel != nil {
		w.cancel()
	}

	if w.events != nil {
		close(w.events)
	}

	w.wg.Wait()

	if w.iface != nil {
		return w.iface.Close()
	}

	return nil
}

// MARK: BatchSize (OptimizedTUNWrapper)
func (w *OptimizedTUNWrapper) BatchSize() int {
	return w.batchSize
}
