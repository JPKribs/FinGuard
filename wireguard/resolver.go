package wireguard

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

// MARK: NewAsyncResolver
// Creates a new asynchronous DNS resolver with workers and cache cleanup
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
// Resolves a hostname to IP addresses asynchronously with optional timeout
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
// Returns a cached IP synchronously if available
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
// Retrieves a cached DNS entry if it exists and is valid
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
// Stores a DNS entry in the cache with a TTL
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
// Resolves queued requests continuously in a background goroutine
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
// Performs the DNS resolution for a single queued request
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
// Periodically removes expired entries from the DNS cache
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
// Deletes stale cache entries exceeding their TTL
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
// Stops the resolver and waits for all workers to finish
func (r *AsyncResolver) Close() {
	r.cancel()
	r.wg.Wait()
}

// MARK: GetStats
// Returns the hit/miss/error/timeout statistics of the resolver
func (r *AsyncResolver) GetStats() (hits, misses, errors, timeouts uint64) {
	return atomic.LoadUint64(&r.stats.hits),
		atomic.LoadUint64(&r.stats.misses),
		atomic.LoadUint64(&r.stats.errors),
		atomic.LoadUint64(&r.stats.timeouts)
}
