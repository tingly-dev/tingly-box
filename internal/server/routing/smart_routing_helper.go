package routing

import (
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

// ExtractRequestContext extracts RequestContext from different request types
func ExtractRequestContext(req interface{}) (*smartrouting.RequestContext, error) {
	return smartrouting.ExtractContext(req), nil
}
