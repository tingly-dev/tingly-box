package protocoltest

// duo_serve.go is the CHILD side of the duo two-process environment: a full
// production tingly-box instance booted via server.Start (background
// refreshers, config watcher, real http.Server timeouts — everything a real
// deployment runs). The parent (NewDuoEnv) re-executes its own binary with
// the typed duoInstanceSpec contract below (one JSON document in the
// TINGLY_DUO_SPEC environment variable); MaybeRunDuoServe intercepts that
// re-execution before normal CLI/test execution begins and never returns.
//
// The child self-seeds its own config dir, so the parent never opens the
// instance's SQLite store — the only cross-process contract is the spec
// plus reading the child's config.json for tokens.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
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

// tb2 provider UUIDs wired by seedDuoGateway; duo_routing.go's scenario
// services reference the same IDs, so they live in one place.
const (
	DuoProviderChat      = "tb1-openai-chat"
	DuoProviderResponses = "tb1-openai-responses"
	DuoProviderAnthropic = "tb1-anthropic"
)

// Slow-stream vmodel IDs registered on tb1 for the backpressure routes.
// Unlike the builtin virtual-gpt-4/virtual-claude-3 (tiny instant response),
// these stream a configurably large response over a configurable duration.
const (
	DuoSlowOpenAIModel    = "duo-slow-gpt"
	DuoSlowAnthropicModel = "duo-slow-claude"
)

// MaybeRunDuoServe runs a duo child instance and exits the process when the
// duo spec is present in the environment; otherwise it returns immediately.
// Call it first thing in main() (cli/harness) and TestMain (duo_test.go) so
// the parent can re-execute the same binary as a server.
func MaybeRunDuoServe() {
	raw := os.Getenv(duoEnvSpec)
	if raw == "" {
		return
	}
	spec, err := decodeDuoSpec(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "duo-serve: %v\n", err)
		os.Exit(1)
	}
	if err := runDuoServe(spec); err != nil {
		fmt.Fprintf(os.Stderr, "duo-serve[%s]: %v\n", spec.Name, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runDuoServe(spec duoInstanceSpec) error {
	appCfg, err := config.NewAppConfig(config.WithConfigDir(spec.ConfigDir))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Gateway role: wire providers + one rule per duo route to the upstream.
	if spec.Role == duoRoleGateway {
		if err := seedDuoGateway(appCfg, spec.UpstreamURL, spec.UpstreamToken); err != nil {
			return fmt.Errorf("seed gateway wiring: %w", err)
		}
		// Pipeline health scenarios need a non-zero recovery window so a 429
		// remains observable by the next request. State still comes exclusively
		// from real gateway traffic; this only makes the duo config explicit and
		// deterministic instead of inheriting a zero-value immediate recovery.
		appCfg.GetGlobalConfig().HealthMonitor = loadbalance.DefaultHealthMonitorConfig()
	}

	// Production boots with a MultiLogger; without it the smart-routing and
	// model-request memory sinks don't exist and /api/v1/requests has nothing
	// to join, so the child would be less observable than a real deployment.
	multiLogger, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(spec.ConfigDir))
	if err != nil {
		return fmt.Errorf("init multi logger: %w", err)
	}

	// Option assembly mirrors the CLI's startServer (command/server.go): the
	// same server.ServerOption vocabulary, minus interactive concerns
	// (browser, banner, file lock) that have no place in a child process.
	// HTTPTimeouts is passed through unconditionally — zero fields keep
	// Start()'s defaults by WithHTTPTimeouts's own contract.
	srv := server.NewServer(appCfg.GetGlobalConfig(),
		server.WithOpenBrowser(false),
		server.WithMultiLogger(multiLogger),
		server.WithHTTPTimeouts(spec.HTTPTimeouts),
	)

	if spec.Role == duoRoleUpstream {
		// Upstream role: register the duo-only vmodels before serving — the
		// slow/large backpressure models and the service-identity pool the
		// routing scenarios address.
		if spec.Stream.SizeKB > 0 {
			if err := registerDuoStreamModels(srv.GetVirtualModelService(), spec.Stream); err != nil {
				return fmt.Errorf("register duo stream models: %w", err)
			}
		}
		if err := registerDuoServiceModels(srv.GetVirtualModelService()); err != nil {
			return fmt.Errorf("register duo service models: %w", err)
		}
	}

	return srv.Start(spec.Port)
}

// seedDuoGateway persists the tb2 wiring into the child's own config dir:
// one provider per target protocol pointing at tb1's /virtual endpoints, and
// one anthropic-scenario rule per duo route (fast and slow variants).
func seedDuoGateway(appCfg *config.AppConfig, tb1URL, tb1Token string) error {
	providers := map[string]*typ.Provider{
		"chat": {
			UUID:               DuoProviderChat,
			Name:               DuoProviderChat,
			APIBase:            tb1URL + "/virtual/openai/v1",
			APIStyle:           protocol.APIStyleOpenAI,
			OpenAIEndpointMode: ai.EndpointModeChat,
		},
		"responses": {
			UUID:               DuoProviderResponses,
			Name:               DuoProviderResponses,
			APIBase:            tb1URL + "/virtual/openai/v1",
			APIStyle:           protocol.APIStyleOpenAI,
			OpenAIEndpointMode: ai.EndpointModeResponses,
		},
		"anthropic": {
			UUID:     DuoProviderAnthropic,
			Name:     DuoProviderAnthropic,
			APIBase:  tb1URL + "/virtual/anthropic", // SDK appends /v1/messages
			APIStyle: protocol.APIStyleAnthropic,
		},
	}
	for _, p := range providers {
		p.Token = tb1Token
		p.Enabled = true
		p.Timeout = int64(constant.DefaultRequestTimeout)
		if err := appCfg.AddProvider(p); err != nil {
			return fmt.Errorf("add provider %s: %w", p.Name, err)
		}
	}

	for _, route := range allDuoRoutesWithSlow() {
		rule := newHarnessRule(route.RequestModel(), typ.ScenarioAnthropic, route.RequestModel(), duoTargetVModel(route),
			harnessService(providers[route.Target].UUID, duoTargetVModel(route)))
		if err := appCfg.GetGlobalConfig().AddRequestConfig(rule); err != nil {
			return fmt.Errorf("add rule %s: %w", route.Name, err)
		}
	}
	return nil
}

// registerDuoStreamModels registers the slow/large vmodels into tb1's
// registries. The response streams len(chunks) deltas of ~2 KB each; the
// shape's Delay semantics are documented on DuoStreamShape.
func registerDuoStreamModels(svc *virtualserver.Service, shape DuoStreamShape) error {
	if svc == nil {
		return fmt.Errorf("virtual model service unavailable")
	}
	chunks := duoStreamChunks(shape.SizeKB)
	content := strings.Join(chunks, "")
	description := fmt.Sprintf("duo backpressure model: ~%d KB streamed over ~%s", shape.SizeKB, 2*shape.Delay)

	if err := svc.GetOpenAIRegistry().Register(openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:           DuoSlowOpenAIModel,
		Name:         "Duo slow GPT",
		Description:  description,
		Content:      content,
		StreamChunks: chunks,
		Delay:        shape.Delay,
	})); err != nil {
		return err
	}
	return svc.GetAnthropicRegistry().Register(anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:           DuoSlowAnthropicModel,
		Name:         "Duo slow Claude",
		Description:  description,
		Content:      content,
		StreamChunks: chunks,
		Delay:        shape.Delay,
	}))
}

// DuoServiceIdentities is the pool of service-identity vmodels registered on
// tb1 for the routing scenarios. Each identity answers with a distinct,
// recognizable marker (see DuoServiceMarker), so which service a request was
// routed to is readable directly from the response body — a wire-level
// assertion that needs no cooperation from the gateway under test.
var DuoServiceIdentities = []string{"a", "b", "c", "d", "e", "f"}

// DuoServiceModel returns the tb1 vmodel ID for a service identity.
func DuoServiceModel(identity string) string { return "duo-svc-" + identity }

// DuoServiceMarker returns the response-content marker a service-identity
// vmodel answers with.
func DuoServiceMarker(identity string) string { return "[duo-svc:" + identity + "]" }

// registerDuoServiceModels registers the service-identity pool into both of
// tb1's registries, so scenario services can target any provider protocol.
func registerDuoServiceModels(svc *virtualserver.Service) error {
	if svc == nil {
		return fmt.Errorf("virtual model service unavailable")
	}
	for _, id := range DuoServiceIdentities {
		content := "routed to " + DuoServiceMarker(id) + " — duo routing scenario response"
		if err := svc.GetOpenAIRegistry().Register(openaivm.NewMockModel(&openaivm.MockModelConfig{
			ID:          DuoServiceModel(id),
			Name:        "Duo service " + id,
			Description: "duo routing service-identity model",
			Content:     content,
		})); err != nil {
			return err
		}
		if err := svc.GetAnthropicRegistry().Register(anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
			ID:          DuoServiceModel(id),
			Name:        "Duo service " + id,
			Description: "duo routing service-identity model",
			Content:     content,
		})); err != nil {
			return err
		}
	}
	return nil
}

// duoStreamChunks builds ~2 KB filler chunks totalling approximately kb KB.
func duoStreamChunks(kb int) []string {
	const chunkBytes = 2 * 1024
	n := kb * 1024 / chunkBytes
	if n < 1 {
		n = 1
	}
	chunk := duoFiller(chunkBytes)
	chunks := make([]string, n)
	for i := range chunks {
		chunks[i] = chunk
	}
	return chunks
}
