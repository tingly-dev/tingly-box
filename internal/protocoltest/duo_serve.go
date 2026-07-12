package protocoltest

// duo_serve.go is the CHILD side of the duo two-process environment: a full
// production tingly-box instance booted via server.Start (background
// refreshers, config watcher, real http.Server timeouts — everything a real
// deployment runs). The parent (NewDuoEnv) re-executes its own binary with
// the TINGLY_DUO_* environment variables below; MaybeRunDuoServe intercepts
// that re-execution before normal CLI/test execution begins and never
// returns.
//
// The child self-seeds its own config dir, so the parent never opens the
// instance's SQLite store — the only cross-process contract is this env
// block plus reading the child's config.json for tokens.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Environment variable contract between the duo parent and child.
const (
	duoEnvRole      = "TINGLY_DUO_ROLE"
	duoEnvName      = "TINGLY_DUO_NAME"
	duoEnvConfigDir = "TINGLY_DUO_CONFIG_DIR"
	duoEnvPort      = "TINGLY_DUO_PORT"

	// Gateway (tb2) wiring: where the upstream instance (tb1) lives.
	duoEnvUpstreamURL   = "TINGLY_DUO_UPSTREAM_URL"
	duoEnvUpstreamToken = "TINGLY_DUO_UPSTREAM_TOKEN"

	// Upstream (tb1) wiring: shape of the slow/large "backpressure" vmodels.
	duoEnvStreamKB = "TINGLY_DUO_STREAM_KB"
	duoEnvStreamMS = "TINGLY_DUO_STREAM_MS"

	duoRoleServe = "serve"
)

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
// duo env contract is present; otherwise it returns immediately. Call it
// first thing in main() (cli/harness) and TestMain (duo_test.go) so the
// parent can re-execute the same binary as a server.
func MaybeRunDuoServe() {
	if os.Getenv(duoEnvRole) != duoRoleServe {
		return
	}
	if err := runDuoServe(); err != nil {
		fmt.Fprintf(os.Stderr, "duo-serve[%s]: %v\n", os.Getenv(duoEnvName), err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runDuoServe() error {
	dir := os.Getenv(duoEnvConfigDir)
	if dir == "" {
		return fmt.Errorf("%s not set", duoEnvConfigDir)
	}
	port, err := strconv.Atoi(os.Getenv(duoEnvPort))
	if err != nil || port <= 0 {
		return fmt.Errorf("invalid %s: %q", duoEnvPort, os.Getenv(duoEnvPort))
	}

	appCfg, err := config.NewAppConfig(config.WithConfigDir(dir))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	isGateway := os.Getenv(duoEnvUpstreamURL) != ""

	// tb2 role: wire providers + one rule per duo route to the upstream (tb1).
	if isGateway {
		if err := seedDuoGateway(appCfg, os.Getenv(duoEnvUpstreamURL), os.Getenv(duoEnvUpstreamToken)); err != nil {
			return fmt.Errorf("seed gateway wiring: %w", err)
		}
	}

	// Production boots with a MultiLogger; without it the smart-routing and
	// model-request memory sinks don't exist and /api/v1/requests has nothing
	// to join, so the child would be less observable than a real deployment.
	multiLogger, err := obs.NewMultiLogger(obs.DefaultMultiLoggerConfig(dir))
	if err != nil {
		return fmt.Errorf("init multi logger: %w", err)
	}

	srv := server.NewServer(appCfg.GetGlobalConfig(),
		server.WithOpenBrowser(false),
		server.WithMultiLogger(multiLogger),
	)

	if !isGateway {
		// tb1 role: register the duo-only vmodels before serving — the
		// slow/large backpressure models and the service-identity pool the
		// routing scenarios address.
		if kb := duoEnvInt(duoEnvStreamKB); kb > 0 {
			if err := registerDuoStreamModels(srv.GetVirtualModelService(), kb, duoEnvInt(duoEnvStreamMS)); err != nil {
				return fmt.Errorf("register duo stream models: %w", err)
			}
		}
		if err := registerDuoServiceModels(srv.GetVirtualModelService()); err != nil {
			return fmt.Errorf("register duo service models: %w", err)
		}
	}

	return srv.Start(port)
}

func duoEnvInt(key string) int {
	n, _ := strconv.Atoi(os.Getenv(key))
	return n
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
// Delay parameter is applied by the virtualserver handler once up front
// (TTFT) and spread again across chunks by the mock's stream loop, so a
// request's wall time is roughly 2×delay.
func registerDuoStreamModels(svc *virtualserver.Service, kb, ms int) error {
	if svc == nil {
		return fmt.Errorf("virtual model service unavailable")
	}
	chunks := duoStreamChunks(kb)
	delay := time.Duration(ms) * time.Millisecond
	content := strings.Join(chunks, "")

	if err := svc.GetOpenAIRegistry().Register(openaivm.NewMockModel(&openaivm.MockModelConfig{
		ID:           DuoSlowOpenAIModel,
		Name:         "Duo slow GPT",
		Description:  fmt.Sprintf("duo backpressure model: ~%d KB streamed over ~%d ms", kb, 2*ms),
		Content:      content,
		StreamChunks: chunks,
		Delay:        delay,
	})); err != nil {
		return err
	}
	return svc.GetAnthropicRegistry().Register(anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{
		ID:           DuoSlowAnthropicModel,
		Name:         "Duo slow Claude",
		Description:  fmt.Sprintf("duo backpressure model: ~%d KB streamed over ~%d ms", kb, 2*ms),
		Content:      content,
		StreamChunks: chunks,
		Delay:        delay,
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
