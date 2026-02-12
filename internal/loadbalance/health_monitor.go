package loadbalance

import (
	"errors"
	"sync"
	"time"
)

// HealthStatus represents the health state of a service
type HealthStatus int

const (
	HealthHealthy HealthStatus = iota // Service is healthy and available
	HealthUnhealthy                    // Service is unhealthy (rate limited, failing)
)

// String returns the string representation of HealthStatus
func (h HealthStatus) String() string {
	switch h {
	case HealthHealthy:
		return "healthy"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// ServiceHealth tracks health information for a single service
type ServiceHealth struct {
	ServiceID         string        // Unique service identifier (provider:model)
	Status            HealthStatus  // Current health status
	LastError         error         // Last error that caused unhealthy state
	LastErrorTime     time.Time     // When the error occurred
	ConsecutiveErrors int           // Count of consecutive errors
	RateLimited       bool          // True if last error was 429
	AuthError         bool          // True if last error was 401/403
	LastHealthCheck   time.Time     // Last time health was checked
	RecoveryTimeout   time.Duration // Time before auto-recovery
	mutex             sync.RWMutex  // Thread safety
}

// HealthProbeFunc is the function type for probing service health
// Returns true if service is healthy, false otherwise
type HealthProbeFunc func(serviceID string) bool

// HealthMonitorConfig holds configuration for health monitoring
type HealthMonitorConfig struct {
	// ConsecutiveErrorThreshold is the number of consecutive errors before marking unhealthy
	ConsecutiveErrorThreshold int `json:"consecutive_error_threshold" yaml:"consecutive_error_threshold"`
	// RecoveryTimeoutSeconds is the time in seconds before auto-recovery
	RecoveryTimeoutSeconds int `json:"recovery_timeout_seconds" yaml:"recovery_timeout_seconds"`
	// ProbeEnabled enables health check probing before marking service healthy
	ProbeEnabled bool `json:"probe_enabled" yaml:"probe_enabled"`
}

// DefaultHealthMonitorConfig returns default configuration
func DefaultHealthMonitorConfig() HealthMonitorConfig {
	return HealthMonitorConfig{
		ConsecutiveErrorThreshold: 3,
		RecoveryTimeoutSeconds:    300, // 5 minutes
		ProbeEnabled:              true,
	}
}

// HealthMonitor manages health status for all services
type HealthMonitor struct {
	services                  map[string]*ServiceHealth // serviceID -> health
	mutex                     sync.RWMutex
	config                    HealthMonitorConfig
	defaultRecoveryTimeout    time.Duration
	consecutiveErrorThreshold int
	probeFunc                 HealthProbeFunc // Optional probe function for recovery checking
}

// NewHealthMonitor creates a new health monitor with the given configuration
func NewHealthMonitor(config HealthMonitorConfig) *HealthMonitor {
	return &HealthMonitor{
		services:                  make(map[string]*ServiceHealth),
		config:                    config,
		defaultRecoveryTimeout:    time.Duration(config.RecoveryTimeoutSeconds) * time.Second,
		consecutiveErrorThreshold: config.ConsecutiveErrorThreshold,
		probeFunc:                 nil, // Can be set via SetProbeFunc
	}
}

// SetProbeFunc sets the probe function for health checking during recovery
func (hm *HealthMonitor) SetProbeFunc(fn HealthProbeFunc) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()
	hm.probeFunc = fn
}

// getOrCreateHealth gets or creates a health record for a service
func (hm *HealthMonitor) getOrCreateHealth(serviceID string) *ServiceHealth {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	if health, exists := hm.services[serviceID]; exists {
		return health
	}

	health := &ServiceHealth{
		ServiceID:       serviceID,
		Status:          HealthHealthy,
		RecoveryTimeout: hm.defaultRecoveryTimeout,
	}
	hm.services[serviceID] = health
	return health
}

// getHealth gets a health record for a service (returns nil if not found)
func (hm *HealthMonitor) getHealth(serviceID string) *ServiceHealth {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	return hm.services[serviceID]
}

// IsHealthy checks if a service is healthy (with time-based recovery and probing)
func (hm *HealthMonitor) IsHealthy(serviceID string) bool {
	health := hm.getHealth(serviceID)
	if health == nil {
		// No health record means healthy by default
		return true
	}

	health.mutex.RLock()
	status := health.Status
	lastErrorTime := health.LastErrorTime
	recoveryTimeout := health.RecoveryTimeout
	health.mutex.RUnlock()

	if status == HealthHealthy {
		return true
	}

	// Check if recovery timeout has elapsed
	if time.Since(lastErrorTime) > recoveryTimeout {
		// Probe before auto-recover (if probing is enabled and probe func is set)
		if hm.config.ProbeEnabled && hm.probeFunc != nil {
			if hm.probeFunc(serviceID) {
				// Probe succeeded, recover the service
				hm.recoverService(serviceID)
				return true
			}
			// Probe failed, extend the timeout
			hm.extendRecoveryTimeout(serviceID)
			return false
		}

		// No probe function set or probing disabled, auto-recover
		hm.recoverService(serviceID)
		return true
	}

	return false
}

// recoverService recovers a service to healthy state
func (hm *HealthMonitor) recoverService(serviceID string) {
	hm.mutex.RLock()
	health, exists := hm.services[serviceID]
	hm.mutex.RUnlock()

	if !exists {
		return
	}

	health.mutex.Lock()
	defer health.mutex.Unlock()

	if health.Status == HealthUnhealthy {
		health.Status = HealthHealthy
		health.RateLimited = false
		health.AuthError = false
		health.ConsecutiveErrors = 0
		health.LastError = nil
	}
}

// extendRecoveryTimeout extends the recovery timeout after a failed probe
func (hm *HealthMonitor) extendRecoveryTimeout(serviceID string) {
	hm.mutex.RLock()
	health, exists := hm.services[serviceID]
	hm.mutex.RUnlock()

	if !exists {
		return
	}

	health.mutex.Lock()
	defer health.mutex.Unlock()

	// Extend timeout by another recovery period (exponential backoff could be added here)
	health.LastErrorTime = time.Now()
}

// ReportRateLimit reports a 429 rate limit error for a service
func (hm *HealthMonitor) ReportRateLimit(serviceID string) {
	health := hm.getOrCreateHealth(serviceID)
	health.mutex.Lock()
	defer health.mutex.Unlock()

	health.Status = HealthUnhealthy
	health.RateLimited = true
	health.LastErrorTime = time.Now()
	health.ConsecutiveErrors = 0 // Reset consecutive errors on 429
	health.LastHealthCheck = time.Now()
}

// ReportAuthError reports a 401/403 auth error for a service
func (hm *HealthMonitor) ReportAuthError(serviceID string, statusCode int) {
	health := hm.getOrCreateHealth(serviceID)
	health.mutex.Lock()
	defer health.mutex.Unlock()

	// Auth errors immediately mark unhealthy (no threshold)
	health.Status = HealthUnhealthy
	health.AuthError = true
	health.LastError = errors.New("auth error")
	health.LastErrorTime = time.Now()
	health.LastHealthCheck = time.Now()
}

// ReportError reports a retryable error for a service
func (hm *HealthMonitor) ReportError(serviceID string, err error) {
	health := hm.getOrCreateHealth(serviceID)
	health.mutex.Lock()
	defer health.mutex.Unlock()

	health.ConsecutiveErrors++
	health.LastError = err
	health.LastErrorTime = time.Now()
	health.LastHealthCheck = time.Now()

	// Only mark unhealthy after threshold
	if health.ConsecutiveErrors >= hm.consecutiveErrorThreshold {
		health.Status = HealthUnhealthy
	}
}

// ReportSuccess reports a successful request for a service
func (hm *HealthMonitor) ReportSuccess(serviceID string) {
	health := hm.getHealth(serviceID)
	if health == nil {
		return
	}

	health.mutex.Lock()
	defer health.mutex.Unlock()

	// Any success immediately recovers
	if health.Status == HealthUnhealthy {
		health.Status = HealthHealthy
		health.RateLimited = false
		health.AuthError = false
		health.ConsecutiveErrors = 0
		health.LastError = nil
	} else {
		// Reset consecutive errors even if already healthy
		health.ConsecutiveErrors = 0
	}
	health.LastHealthCheck = time.Now()
}

// GetHealth returns the health status for a service
func (hm *HealthMonitor) GetHealth(serviceID string) *ServiceHealth {
	return hm.getOrCreateHealth(serviceID)
}

// GetAllHealth returns health status for all services
func (hm *HealthMonitor) GetAllHealth() map[string]*ServiceHealth {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	result := make(map[string]*ServiceHealth, len(hm.services))
	for k, v := range hm.services {
		result[k] = v
	}
	return result
}

// ResetHealth manually resets a service's health to healthy
func (hm *HealthMonitor) ResetHealth(serviceID string) {
	hm.recoverService(serviceID)
}

// RemoveHealth removes a service's health record
func (hm *HealthMonitor) RemoveHealth(serviceID string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	delete(hm.services, serviceID)
}

// UpdateConfig updates the health monitor configuration
func (hm *HealthMonitor) UpdateConfig(config HealthMonitorConfig) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	hm.config = config
	hm.defaultRecoveryTimeout = time.Duration(config.RecoveryTimeoutSeconds) * time.Second
	hm.consecutiveErrorThreshold = config.ConsecutiveErrorThreshold
}
