package loadbalance

import (
	"fmt"
	"sync"
	"time"
)

// SessionTracker tracks active sessions per provider and model.
// It enforces capacity limits at both the model and provider levels,
// and handles idle session cleanup.
type SessionTracker struct {
	mu sync.RWMutex

	// providerSessions tracks current session count per provider (for provider total capacity)
	providerSessions map[string]int64

	// providerTotalCapacities tracks the total capacity per provider
	providerTotalCapacities map[string]int64

	// serviceSessions tracks current session count per service (provider:model)
	serviceSessions map[string]int64

	// serviceCapacities tracks the capacity per service (provider:model)
	serviceCapacities map[string]int64

	// sessions tracks session info for cleanup and lookup
	sessions map[string]*ServiceInfo

	// timeout is the idle timeout for session cleanup
	timeout time.Duration
}

// ServiceInfo tracks a session's state for capacity tracking and cleanup.
type ServiceInfo struct {
	ServiceID      string    // provider:model
	ProviderUUID   string    // Provider UUID
	ModelName      string    // Model name
	CreatedAt      time.Time // When the session was created
	LastActivityAt time.Time // Last activity timestamp for idle cleanup
}

// NewSessionTracker creates a new SessionTracker with the specified idle timeout.
// The timeout is used for cleaning up idle sessions that have not had activity.
func NewSessionTracker(timeout time.Duration) *SessionTracker {
	return &SessionTracker{
		providerSessions:     make(map[string]int64),
		providerTotalCapacities: make(map[string]int64),
		serviceSessions:      make(map[string]int64),
		serviceCapacities:    make(map[string]int64),
		sessions:             make(map[string]*ServiceInfo),
		timeout:              timeout,
	}
}

// InitializeCapacities sets up the capacity limits for services and providers.
// services: list of Service structs with ModelCapacity set
// providerCapacities: map of "providerUUID" -> total capacity and "providerUUID:model" -> model capacity
func (t *SessionTracker) InitializeCapacities(services []*Service, providerCapacities map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Clear existing capacities
	t.providerTotalCapacities = make(map[string]int64)
	t.serviceCapacities = make(map[string]int64)

	// Set service/model capacities
	for _, svc := range services {
		if svc.ModelCapacity != nil {
			serviceID := svc.ServiceID()
			t.serviceCapacities[serviceID] = int64(*svc.ModelCapacity)
		}
	}

	// Set provider total capacities
	for key, capacity := range providerCapacities {
		// Provider total capacity is stored with just the provider UUID
		// Model capacities are stored with "provider:model" format
		if !contains(key, ":") {
			t.providerTotalCapacities[key] = capacity
		}
	}
}

// contains checks if a string contains a colon (used to distinguish provider from service keys)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TryAcquire attempts to acquire a session slot for the given provider and model.
// Returns true if the slot was acquired, false if at capacity.
// Both model-level and provider-level capacities are checked.
func (t *SessionTracker) TryAcquire(sessionID, providerUUID, model string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	serviceID := fmt.Sprintf("%s:%s", providerUUID, model)

	// Check 1: Model capacity
	if modelCap, ok := t.serviceCapacities[serviceID]; ok {
		currentModelSessions := t.serviceSessions[serviceID]
		if currentModelSessions >= modelCap {
			return false, fmt.Errorf("model %s at capacity (%d/%d)", model, currentModelSessions, modelCap)
		}
	}

	// Check 2: Provider total capacity
	if providerCap, ok := t.providerTotalCapacities[providerUUID]; ok {
		currentProviderSessions := t.providerSessions[providerUUID]
		if currentProviderSessions >= providerCap {
			return false, fmt.Errorf("provider at total capacity (%d/%d)", currentProviderSessions, providerCap)
		}
	}

	// Both checks passed, acquire slot
	t.serviceSessions[serviceID]++
	t.providerSessions[providerUUID]++

	t.sessions[sessionID] = &ServiceInfo{
		ServiceID:      serviceID,
		ProviderUUID:   providerUUID,
		ModelName:      model,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}

	return true, nil
}

// Release releases a session slot, freeing capacity for new sessions.
func (t *SessionTracker) Release(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, ok := t.sessions[sessionID]
	if !ok {
		return
	}

	t.serviceSessions[info.ServiceID]--
	t.providerSessions[info.ProviderUUID]--

	delete(t.sessions, sessionID)
}

// RecordActivity updates the last activity timestamp for a session.
// This resets the idle timeout for the session.
func (t *SessionTracker) RecordActivity(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if info, ok := t.sessions[sessionID]; ok {
		info.LastActivityAt = time.Now()
	}
}

// CleanupIdleSessions removes sessions that have been idle beyond the timeout.
// This frees up capacity for new sessions.
func (t *SessionTracker) CleanupIdleSessions() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for sessionID, info := range t.sessions {
		if now.Sub(info.LastActivityAt) > t.timeout {
			t.serviceSessions[info.ServiceID]--
			t.providerSessions[info.ProviderUUID]--
			delete(t.sessions, sessionID)
		}
	}
}

// GetAvailableServices returns services that have available capacity.
// Both model and provider constraints must be satisfied.
func (t *SessionTracker) GetAvailableServices(services []*Service) []*Service {
	t.mu.RLock()
	defer t.mu.RUnlock()

	available := make([]*Service, 0)

	for _, svc := range services {
		if !svc.Active {
			continue
		}

		serviceID := svc.ServiceID()
		providerUUID := svc.Provider

		// Check model capacity
		if modelCap, ok := t.serviceCapacities[serviceID]; ok {
			if t.serviceSessions[serviceID] >= modelCap {
				continue
			}
		}

		// Check provider total capacity
		if providerCap, ok := t.providerTotalCapacities[providerUUID]; ok {
			if t.providerSessions[providerUUID] >= providerCap {
				continue
			}
		}

		available = append(available, svc)
	}

	return available
}

// GetAvailableCapacity returns the remaining capacity for a service.
// Returns the minimum of model remaining and provider remaining (-1 means unlimited).
func (t *SessionTracker) GetAvailableCapacity(svc *Service) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	serviceID := svc.ServiceID()
	providerUUID := svc.Provider

	// Model remaining capacity
	var modelRemaining int64 = -1 // -1 means unlimited
	if modelCap, ok := t.serviceCapacities[serviceID]; ok {
		current := t.serviceSessions[serviceID]
		modelRemaining = modelCap - current
	}

	// Provider remaining capacity
	var providerRemaining int64 = -1
	if providerCap, ok := t.providerTotalCapacities[providerUUID]; ok {
		current := t.providerSessions[providerUUID]
		providerRemaining = providerCap - current
	}

	// Return the smaller value (both constraints must be satisfied)
	if modelRemaining < 0 {
		return providerRemaining
	}
	if providerRemaining < 0 {
		return modelRemaining
	}
	if modelRemaining < providerRemaining {
		return modelRemaining
	}
	return providerRemaining
}

// StartCleanup starts a background goroutine that periodically cleans up idle sessions.
// The interval determines how often CleanupIdleSessions is called.
func (t *SessionTracker) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.CleanupIdleSessions()
			}
		}
	}()
}

// GetSessionInfo returns the session info for a given session ID.
func (t *SessionTracker) GetSessionInfo(sessionID string) *ServiceInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.sessions[sessionID]
}

// GetTotalSessions returns the total number of active sessions.
func (t *SessionTracker) GetTotalSessions() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total int64
	for _, count := range t.providerSessions {
		total += count
	}
	return total
}

// GetTotalCapacity returns the total capacity across all providers.
func (t *SessionTracker) GetTotalCapacity() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total int64
	for _, cap := range t.providerTotalCapacities {
		total += cap
	}
	return total
}

// GetSessionCount returns the number of sessions for a specific provider.
func (t *SessionTracker) GetSessionCount(providerUUID string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.providerSessions[providerUUID]
}

// GetModelSessionCount returns the number of sessions for a specific service.
func (t *SessionTracker) GetModelSessionCount(serviceID string) int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.serviceSessions[serviceID]
}
