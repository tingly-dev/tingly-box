package typ

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// HealthFilter filters services based on their health status
type HealthFilter struct {
	monitor *loadbalance.HealthMonitor
}

// NewHealthFilter creates a new health filter with the given health monitor
func NewHealthFilter(monitor *loadbalance.HealthMonitor) *HealthFilter {
	return &HealthFilter{
		monitor: monitor,
	}
}

// Filter filters out unhealthy services from the given list
// Returns only healthy services. If no healthy services are found,
// returns an empty slice (not nil).
func (hf *HealthFilter) Filter(services []*loadbalance.Service) []*loadbalance.Service {
	if hf.monitor == nil {
		// If no health monitor is configured, treat all services as healthy
		return services
	}

	var healthy []*loadbalance.Service
	for _, svc := range services {
		if svc != nil && hf.monitor.IsHealthy(svc.ServiceID()) {
			healthy = append(healthy, svc)
		}
	}
	return healthy
}

// FilterWithFallback filters services and returns healthy ones.
// If no healthy services are found, returns all services as a fallback.
// This is useful when you want to avoid complete failure even if all
// services are marked unhealthy.
func (hf *HealthFilter) FilterWithFallback(services []*loadbalance.Service) []*loadbalance.Service {
	healthy := hf.Filter(services)
	if len(healthy) == 0 && len(services) > 0 {
		// Fallback: return all services if none are healthy
		return services
	}
	return healthy
}

// IsHealthy checks if a specific service is healthy
func (hf *HealthFilter) IsHealthy(serviceID string) bool {
	if hf.monitor == nil {
		return true
	}
	return hf.monitor.IsHealthy(serviceID)
}

// GetHealthMonitor returns the underlying health monitor
func (hf *HealthFilter) GetHealthMonitor() *loadbalance.HealthMonitor {
	return hf.monitor
}
