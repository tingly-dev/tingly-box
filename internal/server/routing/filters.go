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
