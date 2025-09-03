package discovery

import (
	"time"
)

// MARK: NewServerInfoCache
func NewServerInfoCache(cacheDuration time.Duration) *ServerInfoCache {
	return &ServerInfoCache{
		services: make(map[string]*CachedServerInfo),
		duration: cacheDuration,
	}
}

// MARK: Get
func (c *ServerInfoCache) Get(serviceName string) *SystemInfoResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.services[serviceName]
	if !exists {
		return nil
	}

	if c.duration == 0 || time.Since(cached.Timestamp) < c.duration {
		return cached.Info
	}

	delete(c.services, serviceName)
	return nil
}

// MARK: Set
func (c *ServerInfoCache) Set(serviceName string, info *SystemInfoResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[serviceName] = &CachedServerInfo{
		Info:      info,
		Timestamp: time.Now(),
	}
}

// MARK: Remove
func (c *ServerInfoCache) Remove(serviceName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.services, serviceName)
}

// MARK: Clear
func (c *ServerInfoCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services = make(map[string]*CachedServerInfo)
}
