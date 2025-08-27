package wireguard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/JPKribs/FinGuard/config"
	"github.com/JPKribs/FinGuard/internal"
)

type TunnelStatus struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Interface string `json:"interface"`
	MTU       int    `json:"mtu"`
	Peers     int    `json:"peers"`
	Error     string `json:"error,omitempty"`
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

type Manager struct {
	logger        *internal.Logger
	tunnels       map[string]*Tunnel
	mu            sync.RWMutex
	running       bool
	lastError     error
	retryAttempts int
}

const (
	maxRetryAttempts  = 3
	managerRetryDelay = 5 * time.Second
	shutdownTimeout   = 30 * time.Second
)

// MARK: NewManager

// Creates a new WireGuard tunnel manager with logging and recovery capabilities.
func NewManager(logger *internal.Logger) TunnelManager {
	if logger == nil {
		logger = &internal.Logger{}
	}

	return &Manager{
		logger:  logger,
		tunnels: make(map[string]*Tunnel),
	}
}

// MARK: Start

// Initializes the tunnel manager and starts monitoring for tunnel health.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("tunnel manager already running")
	}

	m.logger.Info("Starting WireGuard tunnel manager")
	m.running = true
	m.lastError = nil
	m.retryAttempts = 0

	go m.healthMonitor(ctx)

	return nil
}

// MARK: Stop

// Gracefully shuts down all tunnels and stops the tunnel manager.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.logger.Info("Stopping WireGuard tunnel manager")

	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	var errors []error
	for name, tunnel := range m.tunnels {
		if err := tunnel.Stop(ctx); err != nil {
			m.logger.Error("Failed to stop tunnel", "name", name, "error", err)
			errors = append(errors, fmt.Errorf("tunnel %s: %w", name, err))
		}
	}

	m.running = false

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping tunnels: %v", errors)
	}

	return nil
}

// MARK: CreateTunnel

// Creates and starts a new WireGuard tunnel with retry logic on failure.
func (m *Manager) CreateTunnel(ctx context.Context, cfg config.TunnelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("tunnel manager not running")
	}

	if cfg.Name == "" {
		return fmt.Errorf("tunnel name cannot be empty")
	}

	if _, exists := m.tunnels[cfg.Name]; exists {
		return fmt.Errorf("tunnel %s already exists", cfg.Name)
	}

	var tunnel *Tunnel
	var err error

	for attempt := 1; attempt <= maxRetryAttempts; attempt++ {
		tunnel, err = NewTunnel(cfg, m.logger)
		if err != nil {
			m.logger.Error("Failed to create tunnel", "name", cfg.Name, "attempt", attempt, "error", err)
			if attempt < maxRetryAttempts {
				time.Sleep(managerRetryDelay)
				continue
			}
			return fmt.Errorf("creating tunnel %s after %d attempts: %w", cfg.Name, maxRetryAttempts, err)
		}

		if err = tunnel.Start(ctx); err != nil {
			m.logger.Error("Failed to start tunnel", "name", cfg.Name, "attempt", attempt, "error", err)
			tunnel.Stop(ctx)
			if attempt < maxRetryAttempts {
				time.Sleep(managerRetryDelay)
				continue
			}
			return fmt.Errorf("starting tunnel %s after %d attempts: %w", cfg.Name, maxRetryAttempts, err)
		}

		break
	}

	m.tunnels[cfg.Name] = tunnel
	m.logger.Info("Created tunnel", "name", cfg.Name)
	return nil
}

// MARK: UpdateTunnel

// Updates an existing tunnel configuration and applies changes safely.
func (m *Manager) UpdateTunnel(ctx context.Context, cfg config.TunnelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("tunnel manager not running")
	}

	tunnel, exists := m.tunnels[cfg.Name]
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

// Safely removes and stops a tunnel with graceful cleanup.
func (m *Manager) DeleteTunnel(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("tunnel manager not running")
	}

	tunnel, exists := m.tunnels[name]
	if !exists {
		return fmt.Errorf("tunnel %s not found", name)
	}

	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	if err := tunnel.Stop(ctx); err != nil {
		m.logger.Error("Failed to stop tunnel during deletion", "name", name, "error", err)
	}

	delete(m.tunnels, name)
	m.logger.Info("Deleted tunnel", "name", name)
	return nil
}

// MARK: Status

// Returns the current status of a specific tunnel with error details.
func (m *Manager) Status(ctx context.Context, name string) (TunnelStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		return TunnelStatus{}, fmt.Errorf("tunnel name cannot be empty")
	}

	tunnel, exists := m.tunnels[name]
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

// Returns status information for all managed tunnels with health indicators.
func (m *Manager) ListTunnels(ctx context.Context) ([]TunnelStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]TunnelStatus, 0, len(m.tunnels))
	for _, tunnel := range m.tunnels {
		status := tunnel.Status(ctx)
		if m.lastError != nil && status.State == "stopped" {
			status.Error = m.lastError.Error()
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// MARK: IsReady

// Reports whether the tunnel manager is running and healthy.
func (m *Manager) IsReady() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running && m.retryAttempts < maxRetryAttempts
}

// MARK: Recover

// Attempts to recover failed tunnels and restore service.
func (m *Manager) Recover(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("tunnel manager not running")
	}

	m.logger.Info("Starting tunnel recovery process")

	var recovered, failed int
	for name, tunnel := range m.tunnels {
		status := tunnel.Status(ctx)
		if status.State == "stopped" {
			m.logger.Info("Attempting to recover tunnel", "name", name)

			if err := tunnel.Start(ctx); err != nil {
				m.logger.Error("Failed to recover tunnel", "name", name, "error", err)
				failed++
			} else {
				m.logger.Info("Successfully recovered tunnel", "name", name)
				recovered++
			}
		}
	}

	if failed > 0 {
		return fmt.Errorf("recovery completed: %d recovered, %d failed", recovered, failed)
	}

	m.retryAttempts = 0
	m.lastError = nil
	m.logger.Info("Tunnel recovery completed successfully", "recovered", recovered)
	return nil
}

// MARK: healthMonitor

// Background monitoring routine that checks tunnel health and triggers recovery.
func (m *Manager) healthMonitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck(ctx)
		}
	}
}

// MARK: performHealthCheck

// Checks the health of all tunnels and triggers recovery if needed.
func (m *Manager) performHealthCheck(ctx context.Context) {
	m.mu.RLock()
	if !m.running {
		m.mu.RUnlock()
		return
	}

	var failedTunnels []string
	for name, tunnel := range m.tunnels {
		status := tunnel.Status(ctx)
		if status.State == "stopped" {
			failedTunnels = append(failedTunnels, name)
		}
	}
	m.mu.RUnlock()

	if len(failedTunnels) > 0 {
		m.logger.Info("Health check found failed tunnels", "count", len(failedTunnels), "tunnels", failedTunnels)

		if m.retryAttempts < maxRetryAttempts {
			m.mu.Lock()
			m.retryAttempts++
			m.mu.Unlock()

			if err := m.Recover(ctx); err != nil {
				m.mu.Lock()
				m.lastError = err
				m.mu.Unlock()
				m.logger.Error("Automatic recovery failed", "error", err)
			}
		} else {
			m.logger.Error("Maximum retry attempts reached, manual intervention required",
				"attempts", m.retryAttempts, "failed_tunnels", failedTunnels)
		}
	}
}
