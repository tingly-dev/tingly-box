package process

import (
	"github.com/tingly-dev/tingly-box/sandbox"
)

// Sandbox asks for the process to run isolated: in the named runtime of the
// sandbox registry, under Policy. How the runtime isolates it is not
// agentboot's business; a runtime that cannot give the policy refuses, and
// the process is not started (sandbox.ErrUnavailable).
type Sandbox struct {
	Runtime string         `json:"runtime"`
	Policy  sandbox.Policy `json:"policy"`
}
