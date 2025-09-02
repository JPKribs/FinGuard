package wireguard

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

const (
	maxRetryAttempts    = 3
	managerRetryDelay   = 2 * time.Second
	shutdownTimeout     = 30 * time.Second
	healthCheckInterval = 15 * time.Second
)

// Manager lifecycle functions

// MARK: NewManager
// Creates a new tunnel manager with logger and resolver
func NewManager(logger *internal.Logger) TunnelManager {
	if logger == nil {
		logger = &internal.Logger{}
	}

	return &Manager{
		logger:   logger,
		tunnels:  make(map[string]*Tunnel),
		resolver: NewAsyncResolver(),
	}
}

// MARK: Start
// Starts the tunnel manager and initializes health monitoring
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if atomic.LoadInt64(&m.running) == 1 {
		return fmt.Errorf("tunnel manager already running")
	}

	m.logger.Info("Starting optimized WireGuard tunnel manager")
	atomic.StoreInt64(&m.running, 1)
	m.lastError = nil
	atomic.StoreInt32(&m.retryAttempts, 0)

	m.ctx, m.cancel = context.WithCancel(ctx)

	m.wg.Add(1)
	go m.healthMonitor()

	return nil
}

// MARK: Stop
// Stops the tunnel manager and all active tunnels
func (m *Manager) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt64(&m.running, 1, 0) {
		return nil
	}

	m.logger.Info("Stopping WireGuard tunnel manager")

	if m.cancel != nil {
		m.cancel()
	}

	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	m.mu.Lock()
	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	m.mu.Unlock()

	var errors []error
	for _, tunnel := range tunnels {
		if err := tunnel.Stop(ctx); err != nil {
			m.logger.Error("Failed to stop tunnel", "name", tunnel.name, "error", err)
			errors = append(errors, fmt.Errorf("tunnel %s: %w", tunnel.name, err))
		}
	}

	m.wg.Wait()

	if m.resolver != nil {
		m.resolver.Close()
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping tunnels: %v", errors)
	}

	return nil
}

// Tunnel management functions

// MARK: CreateTunnel
// Creates a new tunnel with retry logic and starts it
func (m *Manager) CreateTunnel(ctx context.Context, cfg config.TunnelConfig) error {
	if atomic.LoadInt64(&m.running) == 0 {
		return fmt.Errorf("tunnel manager not running")
	}

	if cfg.Name == "" {
		return fmt.Errorf("tunnel name cannot be empty")
	}

	m.mu.Lock()
	if _, exists := m.tunnels[cfg.Name]; exists {
		m.mu.Unlock()
		return fmt.Errorf("tunnel %s already exists", cfg.Name)
	}
	m.mu.Unlock()

	var tunnel *Tunnel
	var err error

	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		tunnel, err = NewTunnel(cfg, m.logger, m.resolver)
		if err != nil {
			m.logger.Error("Failed to create tunnel", "name", cfg.Name, "attempt", attempt, "error", err)
			if attempt < maxRetryAttempts {
				time.Sleep(managerRetryDelay * time.Duration(attempt))
				continue
			}
			return fmt.Errorf("creating tunnel %s after %d attempts: %w", cfg.Name, maxRetryAttempts, err)
		}

		if err = tunnel.Start(ctx); err != nil {
			m.logger.Error("Failed to start tunnel", "name", cfg.Name, "attempt", attempt, "error", err)
			tunnel.Stop(ctx)
			if attempt < maxRetryAttempts {
				time.Sleep(managerRetryDelay * time.Duration(attempt))
				continue
			}
			return fmt.Errorf("starting tunnel %s after %d attempts: %w", cfg.Name, maxRetryAttempts, err)
		}

		break
	}

	m.mu.Lock()
	m.tunnels[cfg.Name] = tunnel
	m.mu.Unlock()

	m.logger.Info("Created optimized tunnel", "name", cfg.Name)
	return nil
}

// MARK: UpdateTunnel
// Updates an existing tunnel configuration with rollback on failure
func (m *Manager) UpdateTunnel(ctx context.Context, cfg config.TunnelConfig) error {
	if atomic.LoadInt64(&m.running) == 0 {
		return fmt.Errorf("tunnel manager not running")
	}

	m.mu.RLock()
	tunnel, exists := m.tunnels[cfg.Name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("tunnel %s not found", cfg.Name)
	}

	oldConfig := tunnel.config
	if err := tunnel.Update(ctx, cfg); err != nil {
		m.logger.Error("Failed to update tunnel, attempting rollback", "name", cfg.Name, "error", err)

		if rollbackErr := tunnel.Update(ctx, oldConfig); rollbackErr != nil {
			m.logger.Error("Rollback failed", "name", cfg.Name, "error", rollbackErr)
			return fmt.Errorf("update failed and rollback failed: %w, rollback error: %v", err, rollbackErr)
		}

		return fmt.Errorf("updating tunnel %s: %w", cfg.Name, err)
	}

	m.logger.Info("Updated tunnel", "name", cfg.Name)
	return nil
}

// MARK: DeleteTunnel
// Deletes a tunnel and cleans up its resources
func (m *Manager) DeleteTunnel(ctx context.Context, name string) error {
	if atomic.LoadInt64(&m.running) == 0 {
		return fmt.Errorf("tunnel manager not running")
	}

	m.mu.Lock()
	tunnel, exists := m.tunnels[name]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("tunnel %s not found", name)
	}
	delete(m.tunnels, name)
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	if err := tunnel.Stop(ctx); err != nil {
		m.logger.Error("Failed to stop tunnel during deletion", "name", name, "error", err)
	}

	m.logger.Info("Deleted tunnel", "name", name)
	return nil
}

// Status and monitoring functions

// MARK: Status
// Returns the status of a specific tunnel
func (m *Manager) Status(ctx context.Context, name string) (TunnelStatus, error) {
	if name == "" {
		return TunnelStatus{}, fmt.Errorf("tunnel name cannot be empty")
	}

	m.mu.RLock()
	tunnel, exists := m.tunnels[name]
	m.mu.RUnlock()

	if !exists {
		return TunnelStatus{}, fmt.Errorf("tunnel %s not found", name)
	}

	status := tunnel.Status(ctx)
	if m.lastError != nil && status.State == "stopped" {
		status.Error = m.lastError.Error()
	}

	return status, nil
}

// MARK: ListTunnels
// Returns status information for all tunnels
func (m *Manager) ListTunnels(ctx context.Context) ([]TunnelStatus, error) {
	m.mu.RLock()
	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	m.mu.RUnlock()

	statuses := make([]TunnelStatus, 0, len(tunnels))
	for _, tunnel := range tunnels {
		status := tunnel.Status(ctx)
		if m.lastError != nil && status.State == "stopped" {
			status.Error = m.lastError.Error()
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// MARK: IsReady
// Checks if the tunnel manager is ready to accept operations
func (m *Manager) IsReady() bool {
	return atomic.LoadInt64(&m.running) == 1 && atomic.LoadInt32(&m.retryAttempts) < maxRetryAttempts
}

// Recovery and health functions

// MARK: Recover
// Attempts to recover failed tunnels
func (m *Manager) Recover(ctx context.Context) error {
	if atomic.LoadInt64(&m.running) == 0 {
		return fmt.Errorf("tunnel manager not running")
	}

	m.logger.Info("Starting tunnel recovery process")

	m.mu.RLock()
	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	m.mu.RUnlock()

	var recovered, failed int
	for _, tunnel := range tunnels {
		status := tunnel.Status(ctx)
		if status.State == "stopped" {
			m.logger.Info("Attempting to recover tunnel", "name", tunnel.name)

			if err := tunnel.Start(ctx); err != nil {
				m.logger.Error("Failed to recover tunnel", "name", tunnel.name, "error", err)
				failed++
			} else {
				m.logger.Info("Successfully recovered tunnel", "name", tunnel.name)
				recovered++
			}
		}
	}

	if failed > 0 {
		return fmt.Errorf("recovery completed: %d recovered, %d failed", recovered, failed)
	}

	atomic.StoreInt32(&m.retryAttempts, 0)
	m.lastError = nil
	m.logger.Info("Tunnel recovery completed successfully", "recovered", recovered)
	return nil
}

// MARK: healthMonitor
// Continuous health monitoring routine
func (m *Manager) healthMonitor() {
	defer m.wg.Done()

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// MARK: performHealthCheck
// Performs health checks on all tunnels and triggers recovery if needed
func (m *Manager) performHealthCheck() {
	if atomic.LoadInt64(&m.running) == 0 {
		return
	}

	m.mu.RLock()
	tunnels := make([]*Tunnel, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	m.mu.RUnlock()

	var failedTunnels []string
	for _, tunnel := range tunnels {
		status := tunnel.Status(m.ctx)
		if status.State == "stopped" {
			failedTunnels = append(failedTunnels, tunnel.name)
		}
	}

	if len(failedTunnels) > 0 {
		retryCount := atomic.LoadInt32(&m.retryAttempts)
		if retryCount < maxRetryAttempts {
			m.logger.Info("Health check found failed tunnels", "count", len(failedTunnels), "tunnels", failedTunnels)

			atomic.AddInt32(&m.retryAttempts, 1)

			if err := m.Recover(m.ctx); err != nil {
				m.lastError = err
				m.logger.Error("Automatic recovery failed", "error", err)
			}
		} else {
			m.logger.Error("Maximum retry attempts reached, manual intervention required",
				"attempts", retryCount, "failed_tunnels", failedTunnels)
		}
	}
}
