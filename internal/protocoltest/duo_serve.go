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
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/obs"
	"github.com/tingly-dev/tingly-box/vmodel"
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

// Truncating vmodel IDs registered on tb1 for the Codex passthrough
// truncation scenarios (#1384): both stream a few deltas and then break the
// stream without a terminal event — CleanEOF ends the chunked body properly
// (upstream gateway timeout shape), Drop hijacks and closes the TCP
// connection (provider crash shape).
const (
	DuoTruncEOFVModel  = "duo-trunc-eof-gpt"
	DuoTruncDropVModel = "duo-trunc-drop-gpt"
)

// tb2 request models for the Codex (OpenAI Responses passthrough) routes.
// Scenario is "codex", the surface Codex CLI actually uses.
const (
	DuoCodexOKModel        = "duo-e2e-codex-ok"
	DuoCodexTruncEOFModel  = "duo-e2e-codex-trunc-eof"
	DuoCodexTruncDropModel = "duo-e2e-codex-trunc-drop"
)

// duoCodexRules maps each codex-scenario request model to the tb1 vmodel
// backing it via the responses (passthrough) provider.
func duoCodexRules() map[string]string {
	return map[string]string{
		DuoCodexOKModel:        "virtual-gpt-4",
		DuoCodexTruncEOFModel:  DuoTruncEOFVModel,
		DuoCodexTruncDropModel: DuoTruncDropVModel,
	}
}

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
		// Pipeline health scenarios need a non-zero recovery window so a 429
		// remains observable by the next request. State still comes exclusively
		// from real gateway traffic; this only makes the duo config explicit and
		// deterministic instead of inheriting a zero-value immediate recovery.
		appCfg.GetGlobalConfig().HealthMonitor = loadbalance.DefaultHealthMonitorConfig()
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
		if err := registerDuoTruncationModels(srv.GetVirtualModelService()); err != nil {
			return fmt.Errorf("register duo truncation models: %w", err)
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

	// Codex passthrough routes: codex-scenario rules through the responses
	// provider, one per truncation shape plus the healthy control.
	for reqModel, vm := range duoCodexRules() {
		rule := newHarnessRule(reqModel, typ.ScenarioCodex, reqModel, vm,
			harnessService(DuoProviderResponses, vm))
		if err := appCfg.GetGlobalConfig().AddRequestConfig(rule); err != nil {
			return fmt.Errorf("add rule %s: %w", reqModel, err)
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

// registerDuoTruncationModels registers the two truncating vmodels into tb1's
// openai registry: both emit a handful of real deltas, then the virtualserver
// handler breaks the stream per the injection's MidStreamMode — no terminal
// event either way. Exercised by the Codex passthrough truncation routes.
func registerDuoTruncationModels(svc *virtualserver.Service) error {
	if svc == nil {
		return fmt.Errorf("virtual model service unavailable")
	}
	chunks := []string{"The ", "answer ", "is ", "being ", "truncated ", "here"}
	for _, m := range []struct {
		id   string
		mode vmodel.MidStreamMode
		desc string
	}{
		{DuoTruncEOFVModel, vmodel.MidStreamModeCleanEOF, "clean EOF without terminal event"},
		{DuoTruncDropVModel, vmodel.MidStreamModeConnectionClose, "abrupt TCP close mid-stream"},
	} {
		if err := svc.GetOpenAIRegistry().Register(openaivm.NewMockModel(&openaivm.MockModelConfig{
			ID:           m.id,
			Name:         "Duo truncation " + m.id,
			Description:  "duo truncation model: " + m.desc,
			Content:      strings.Join(chunks, ""),
			StreamChunks: chunks,
			Error: &vmodel.ErrorInjection{
				Stage:         vmodel.ErrorStageMidStream,
				MidStreamMode: m.mode,
				AfterEvents:   3,
			},
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
