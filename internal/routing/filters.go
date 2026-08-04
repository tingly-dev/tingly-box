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

// ContainsService checks if target exists in services by service ID.
func ContainsService(services []*loadbalance.Service, target *loadbalance.Service) bool {
	if target == nil {
		return false
	}
	for _, service := range services {
		if service != nil && service.ServiceID() == target.ServiceID() {
			return true
		}
	}
	return false
}

// IntersectServices keeps services that are present in both lists.
func IntersectServices(left, right []*loadbalance.Service) []*loadbalance.Service {
	if len(left) == 0 || len(right) == 0 {
		return []*loadbalance.Service{}
	}

	allowed := make(map[string]struct{}, len(right))
	for _, svc := range right {
		if svc == nil {
			continue
		}
		allowed[svc.ServiceID()] = struct{}{}
	}

	var out []*loadbalance.Service
	for _, svc := range left {
		if svc == nil {
			continue
		}
		if _, ok := allowed[svc.ServiceID()]; ok {
			out = append(out, svc)
		}
	}
	return out
}
