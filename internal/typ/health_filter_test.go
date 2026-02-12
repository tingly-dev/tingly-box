package typ

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

func TestNewHealthFilter(t *testing.T) {
	// Test with nil monitor
	hf := NewHealthFilter(nil)
	assert.NotNil(t, hf)
	assert.Nil(t, hf.monitor)
}

func TestHealthFilter_Filter_WithNilMonitor(t *testing.T) {
	hf := NewHealthFilter(nil)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// With nil monitor, all services should be returned
	filtered := hf.Filter(services)
	assert.Len(t, filtered, 2)
	assert.Equal(t, services, filtered)
}

func TestHealthFilter_Filter_AllHealthy(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// All services should be healthy by default
	filtered := hf.Filter(services)
	assert.Len(t, filtered, 2)
}

func TestHealthFilter_Filter_WithUnhealthy(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// Mark one service as unhealthy
	monitor.ReportRateLimit("p1:m1")

	filtered := hf.Filter(services)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "p2", filtered[0].Provider)
}

func TestHealthFilter_Filter_AllUnhealthy(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// Mark all services as unhealthy
	monitor.ReportRateLimit("p1:m1")
	monitor.ReportRateLimit("p2:m2")

	filtered := hf.Filter(services)
	assert.Len(t, filtered, 0)
}

func TestHealthFilter_Filter_EmptyInput(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	filtered := hf.Filter([]*loadbalance.Service{})
	assert.Len(t, filtered, 0)
}

func TestHealthFilter_Filter_WithNilService(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		nil,
		{Provider: "p2", Model: "m2", Active: true},
	}

	filtered := hf.Filter(services)
	assert.Len(t, filtered, 2)
}

func TestHealthFilter_FilterWithFallback(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// Mark all services as unhealthy
	monitor.ReportRateLimit("p1:m1")
	monitor.ReportRateLimit("p2:m2")

	// FilterWithFallback should return all services when none are healthy
	filtered := hf.FilterWithFallback(services)
	assert.Len(t, filtered, 2)
}

func TestHealthFilter_FilterWithFallback_HealthyExist(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	services := []*loadbalance.Service{
		{Provider: "p1", Model: "m1", Active: true},
		{Provider: "p2", Model: "m2", Active: true},
	}

	// Mark only one service as unhealthy
	monitor.ReportRateLimit("p1:m1")

	// FilterWithFallback should return only healthy services
	filtered := hf.FilterWithFallback(services)
	assert.Len(t, filtered, 1)
	assert.Equal(t, "p2", filtered[0].Provider)
}

func TestHealthFilter_IsHealthy(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	// Mark service as unhealthy
	monitor.ReportRateLimit("p1:m1")

	assert.False(t, hf.IsHealthy("p1:m1"))
	assert.True(t, hf.IsHealthy("p2:m2"))
}

func TestHealthFilter_IsHealthy_NilMonitor(t *testing.T) {
	hf := NewHealthFilter(nil)

	// With nil monitor, everything is healthy
	assert.True(t, hf.IsHealthy("p1:m1"))
	assert.True(t, hf.IsHealthy("p2:m2"))
}

func TestHealthFilter_GetHealthMonitor(t *testing.T) {
	config := loadbalance.DefaultHealthMonitorConfig()
	monitor := loadbalance.NewHealthMonitor(config)
	hf := NewHealthFilter(monitor)

	assert.Equal(t, monitor, hf.GetHealthMonitor())
}
