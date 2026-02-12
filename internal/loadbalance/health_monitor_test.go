package loadbalance

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHealthMonitor(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	assert.NotNil(t, hm)
	assert.NotNil(t, hm.services)
	assert.Equal(t, 3, hm.consecutiveErrorThreshold)
	assert.Equal(t, 5*time.Minute, hm.defaultRecoveryTimeout)
}

func TestHealthMonitor_IsHealthy_Default(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	// Service with no health record should be healthy by default
	assert.True(t, hm.IsHealthy("provider:model"))
}

func TestHealthMonitor_ReportRateLimit(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Initially healthy
	assert.True(t, hm.IsHealthy(serviceID))

	// Report rate limit
	hm.ReportRateLimit(serviceID)

	// Should be unhealthy
	assert.False(t, hm.IsHealthy(serviceID))

	// Check health record
	health := hm.GetHealth(serviceID)
	assert.Equal(t, HealthUnhealthy, health.Status)
	assert.True(t, health.RateLimited)
	assert.False(t, health.AuthError)
	assert.Equal(t, 0, health.ConsecutiveErrors)
}

func TestHealthMonitor_ReportAuthError(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Initially healthy
	assert.True(t, hm.IsHealthy(serviceID))

	// Report auth error (401)
	hm.ReportAuthError(serviceID, 401)

	// Should be unhealthy immediately
	assert.False(t, hm.IsHealthy(serviceID))

	// Check health record
	health := hm.GetHealth(serviceID)
	assert.Equal(t, HealthUnhealthy, health.Status)
	assert.False(t, health.RateLimited)
	assert.True(t, health.AuthError)

	// Reset and test 403
	hm.ResetHealth(serviceID)
	hm.ReportAuthError(serviceID, 403)
	assert.False(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_ReportError_ConsecutiveThreshold(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.ConsecutiveErrorThreshold = 3
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"
	testErr := errors.New("connection timeout")

	// Initially healthy
	assert.True(t, hm.IsHealthy(serviceID))

	// Report 1 error - should still be healthy
	hm.ReportError(serviceID, testErr)
	assert.True(t, hm.IsHealthy(serviceID))

	// Report 2nd error - should still be healthy
	hm.ReportError(serviceID, testErr)
	assert.True(t, hm.IsHealthy(serviceID))

	// Report 3rd error - should now be unhealthy
	hm.ReportError(serviceID, testErr)
	assert.False(t, hm.IsHealthy(serviceID))

	// Check health record
	health := hm.GetHealth(serviceID)
	assert.Equal(t, 3, health.ConsecutiveErrors)
}

func TestHealthMonitor_ReportSuccess_ImmediateRecovery(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Report success
	hm.ReportSuccess(serviceID)

	// Should be healthy again
	assert.True(t, hm.IsHealthy(serviceID))

	// Check health record
	health := hm.GetHealth(serviceID)
	assert.Equal(t, HealthHealthy, health.Status)
	assert.False(t, health.RateLimited)
	assert.False(t, health.AuthError)
	assert.Equal(t, 0, health.ConsecutiveErrors)
}

func TestHealthMonitor_ReportSuccess_ResetsConsecutiveErrors(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.ConsecutiveErrorThreshold = 3
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"
	testErr := errors.New("connection timeout")

	// Report 2 errors
	hm.ReportError(serviceID, testErr)
	hm.ReportError(serviceID, testErr)

	health := hm.GetHealth(serviceID)
	assert.Equal(t, 2, health.ConsecutiveErrors)

	// Report success
	hm.ReportSuccess(serviceID)

	// Consecutive errors should be reset
	health = hm.GetHealth(serviceID)
	assert.Equal(t, 0, health.ConsecutiveErrors)
	assert.True(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_TimeBasedRecovery(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.RecoveryTimeoutSeconds = 1 // 1 second for testing
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Wait for recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Should be healthy again due to time-based recovery
	assert.True(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_ResetHealth(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Manual reset
	hm.ResetHealth(serviceID)

	// Should be healthy
	assert.True(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_RemoveHealth(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Remove health record
	hm.RemoveHealth(serviceID)

	// Should be healthy (no record = default healthy)
	assert.True(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_ConcurrentAccess(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"
	var wg sync.WaitGroup

	// Concurrent error reporting
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hm.ReportError(serviceID, errors.New("test error"))
		}()
	}

	// Concurrent success reporting
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hm.ReportSuccess(serviceID)
		}()
	}

	// Concurrent health checks
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = hm.IsHealthy(serviceID)
		}()
	}

	wg.Wait()

	// Should not panic and should have valid state
	health := hm.GetHealth(serviceID)
	assert.NotNil(t, health)
	// After concurrent success reports, it should be healthy
	assert.True(t, hm.IsHealthy(serviceID))
}

func TestHealthMonitor_GetAllHealth(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	// Create health records for multiple services
	hm.ReportRateLimit("provider-a:gpt-4o")
	hm.ReportRateLimit("provider-b:gpt-4o")
	hm.ReportSuccess("provider-a:gpt-4o") // Make this one healthy

	allHealth := hm.GetAllHealth()

	assert.Len(t, allHealth, 2)
	assert.Contains(t, allHealth, "provider-a:gpt-4o")
	assert.Contains(t, allHealth, "provider-b:gpt-4o")
}

func TestHealthMonitor_UpdateConfig(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	assert.Equal(t, 3, hm.consecutiveErrorThreshold)
	assert.Equal(t, 5*time.Minute, hm.defaultRecoveryTimeout)

	newConfig := HealthMonitorConfig{
		ConsecutiveErrorThreshold: 5,
		RecoveryTimeoutSeconds:    600,
	}
	hm.UpdateConfig(newConfig)

	assert.Equal(t, 5, hm.consecutiveErrorThreshold)
	assert.Equal(t, 10*time.Minute, hm.defaultRecoveryTimeout)
}

func TestHealthStatus_String(t *testing.T) {
	assert.Equal(t, "healthy", HealthHealthy.String())
	assert.Equal(t, "unhealthy", HealthUnhealthy.String())
	assert.Equal(t, "unknown", HealthStatus(999).String())
}

func TestHealthMonitor_ReportError_WhileUnhealthy(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"
	testErr := errors.New("connection timeout")

	// Make service unhealthy via rate limit
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Consecutive errors should be 0 after rate limit
	health := hm.GetHealth(serviceID)
	assert.Equal(t, 0, health.ConsecutiveErrors)

	// Report error while unhealthy
	hm.ReportError(serviceID, testErr)

	// Should increment consecutive errors even while unhealthy
	health = hm.GetHealth(serviceID)
	assert.Equal(t, 1, health.ConsecutiveErrors)
	assert.False(t, hm.IsHealthy(serviceID))
}

// Test probe function that returns true (healthy) by default
var testProbeResult = true

func TestHealthMonitor_ProbeBeforeRecovery(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.ProbeEnabled = true
	config.RecoveryTimeoutSeconds = 1 // 1 second for testing
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Set probe function that checks testProbeResult
	hm.SetProbeFunc(func(sid string) bool {
		return testProbeResult
	})

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Set probe to fail (service still unhealthy)
	testProbeResult = false

	// Wait for recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Should still be unhealthy because probe failed (and timeout extended)
	assert.False(t, hm.IsHealthy(serviceID), "Service should remain unhealthy when probe fails")

	// Set probe to succeed
	testProbeResult = true

	// Wait for recovery timeout again (since it was extended)
	time.Sleep(1100 * time.Millisecond)

	// Should now be healthy because probe succeeded
	assert.True(t, hm.IsHealthy(serviceID), "Service should become healthy when probe succeeds")
}

func TestHealthMonitor_ProbeDisabled(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.ProbeEnabled = false // Probing disabled
	config.RecoveryTimeoutSeconds = 1
	hm := NewHealthMonitor(config)

	// Set probe function that returns false - but it shouldn't be called
	hm.SetProbeFunc(func(sid string) bool {
		return false
	})

	serviceID := "provider-a:gpt-4o"

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Wait for recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Should auto-recover because probing is disabled
	assert.True(t, hm.IsHealthy(serviceID), "Service should auto-recover when probing is disabled")
}

func TestHealthMonitor_ProbeExtendsTimeout(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	config.ProbeEnabled = true
	config.RecoveryTimeoutSeconds = 1
	hm := NewHealthMonitor(config)

	serviceID := "provider-a:gpt-4o"

	// Set probe function
	hm.SetProbeFunc(func(sid string) bool {
		return testProbeResult
	})

	// Make service unhealthy
	hm.ReportRateLimit(serviceID)
	assert.False(t, hm.IsHealthy(serviceID))

	// Set probe to fail
	testProbeResult = false

	// Wait for initial recovery timeout
	time.Sleep(1100 * time.Millisecond)

	// Probe fails, timeout should be extended
	assert.False(t, hm.IsHealthy(serviceID))

	// Wait another second for the extended timeout
	time.Sleep(1100 * time.Millisecond)

	// Probe still fails, should extend timeout again
	assert.False(t, hm.IsHealthy(serviceID))

	// Wait another second and now make probe succeed
	time.Sleep(1100 * time.Millisecond)

	// Now make probe succeed
	testProbeResult = true

	// Wait for timeout and probe to succeed
	time.Sleep(1100 * time.Millisecond)

	// Should now recover
	assert.True(t, hm.IsHealthy(serviceID))
}
