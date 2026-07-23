package protocoltest

// duo_spec.go defines the typed boot contract between the duo parent
// (NewDuoEnv, duo.go) and a child instance (MaybeRunDuoServe, duo_serve.go):
// one JSON-encoded duoInstanceSpec carried in a single environment variable.
// Behaviour is pinned by duo_spec_test.go without booting any process.

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/server"
)

// duoEnvSpec is the single environment variable of the parent→child boot
// contract: a JSON-encoded duoInstanceSpec. Its presence marks the process
// as a duo child.
const duoEnvSpec = "TINGLY_DUO_SPEC"

// duoRole selects which of the two duo roles a child instance plays. The
// role is explicit in the spec rather than inferred from which wiring
// fields happen to be set.
type duoRole string

const (
	// duoRoleGateway is tb2: the gateway under test — converts client
	// requests and proxies them to the upstream instance.
	duoRoleGateway duoRole = "gateway"
	// duoRoleUpstream is tb1: serves the /virtual vmodel endpoints the
	// gateway's providers point at.
	duoRoleUpstream duoRole = "upstream"
)

// duoInstanceSpec is the typed boot contract between the duo parent and one
// child instance — the duo analogue of the CLI's options.StartServerOptions,
// so tb boot parameters read like every other server boot in the codebase
// instead of a stringly env map. The parent fills it in NewDuoEnv /
// startInstance; the child decodes and validates it once in MaybeRunDuoServe.
type duoInstanceSpec struct {
	Name      string  `json:"name"`
	Role      duoRole `json:"role"`
	ConfigDir string  `json:"config_dir"`
	Port      int     `json:"port"`

	// Gateway (tb2) wiring: where the upstream instance (tb1) lives.
	UpstreamURL   string `json:"upstream_url,omitempty"`
	UpstreamToken string `json:"upstream_token,omitempty"`

	// HTTPTimeouts overrides the child's real http.Server timeouts — the
	// server's own packaged type (server.WithHTTPTimeouts), so all four
	// deadlines are configurable here exactly as they are on a production
	// boot; zero fields keep Start()'s defaults. Currently only wired to the
	// gateway role (NewDuoEnv): #1384 is about the gateway's own outbound
	// write to the client, not tb1's.
	HTTPTimeouts server.HTTPTimeouts `json:"http_timeouts"`

	// Stream shapes the slow/large backpressure vmodels (upstream role).
	Stream DuoStreamShape `json:"stream"`
}

// DuoStreamShape parameterizes tb1's slow backpressure vmodels: an
// approximately SizeKB-sized response whose Delay is applied once as TTFT by
// the virtualserver handler and spread again across chunks by the mock's
// stream loop, so a request's wall time is roughly 2×Delay.
type DuoStreamShape struct {
	SizeKB int           `json:"size_kb,omitempty"`
	Delay  time.Duration `json:"delay,omitempty"`
}

// validate rejects a spec that cannot boot, with the field name in the
// error — the earlier env-map contract silently defaulted malformed values.
func (s *duoInstanceSpec) validate() error {
	var missing []string
	if s.Name == "" {
		missing = append(missing, "name")
	}
	if s.ConfigDir == "" {
		missing = append(missing, "config_dir")
	}
	if s.Port <= 0 {
		missing = append(missing, "port")
	}
	switch s.Role {
	case duoRoleGateway:
		if s.UpstreamURL == "" {
			missing = append(missing, "upstream_url")
		}
		if s.UpstreamToken == "" {
			missing = append(missing, "upstream_token")
		}
	case duoRoleUpstream:
	default:
		return fmt.Errorf("unknown duo role %q (want %q or %q)", s.Role, duoRoleGateway, duoRoleUpstream)
	}
	if len(missing) > 0 {
		return fmt.Errorf("duo spec for role %q missing/invalid: %s", s.Role, strings.Join(missing, ", "))
	}
	return nil
}

// encode serializes the spec into the env entry startInstance hands to the
// child process.
func (s *duoInstanceSpec) encode() (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("encode duo spec: %w", err)
	}
	return duoEnvSpec + "=" + string(raw), nil
}

// decodeDuoSpec parses and validates the child-side spec.
func decodeDuoSpec(raw string) (duoInstanceSpec, error) {
	var spec duoInstanceSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return spec, fmt.Errorf("decode %s: %w", duoEnvSpec, err)
	}
	if err := spec.validate(); err != nil {
		return spec, err
	}
	return spec, nil
}
