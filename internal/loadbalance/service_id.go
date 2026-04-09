package loadbalance

import "fmt"

// ServiceID uniquely identifies a provider+model combination in load balancing.
type ServiceID struct {
	ProviderUUID string `json:"provider_uuid"`
	Model        string `json:"model"`
}

// String returns a stable string for use as map key and logging.
func (id ServiceID) String() string {
	return fmt.Sprintf("%s/%s", id.ProviderUUID, id.Model)
}

// NewServiceID creates a ServiceID from provider UUID/name and model.
func NewServiceID(providerUUID, model string) ServiceID {
	return ServiceID{ProviderUUID: providerUUID, Model: model}
}
