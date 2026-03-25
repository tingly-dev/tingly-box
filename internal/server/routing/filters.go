package routing

import (
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// FilterActiveServices returns only active services from the input list
func FilterActiveServices(services []*loadbalance.Service) []*loadbalance.Service {
	if len(services) == 0 {
		return services
	}

	var activeServices []*loadbalance.Service
	for _, service := range services {
		if service.Active {
			activeServices = append(activeServices, service)
		}
	}

	return activeServices
}

// FilterHealthyServices returns only healthy services using the health filter
func FilterHealthyServices(services []*loadbalance.Service, healthFilter interface{}) []*loadbalance.Service {
	if len(services) == 0 {
		return services
	}

	// Type assert to health filter interface
	// TODO: Define proper interface in loadbalance package
	if filter, ok := healthFilter.(interface {
		Filter([]*loadbalance.Service) []*loadbalance.Service
	}); ok {
		return filter.Filter(services)
	}

	// Fallback: return all services if no health filter available
	return services
}
