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

	// tb2 role: wire providers + one rule per duo route to the upstream (tb1).
	if upstreamURL := os.Getenv(duoEnvUpstreamURL); upstreamURL != "" {
		if err := seedDuoGateway(appCfg, upstreamURL, os.Getenv(duoEnvUpstreamToken)); err != nil {
			return fmt.Errorf("seed gateway wiring: %w", err)
		}
	}

	srv := server.NewServer(appCfg.GetGlobalConfig(), server.WithOpenBrowser(false))

	// tb1 role: register the slow/large backpressure vmodels before serving.
	if kb := duoEnvInt(duoEnvStreamKB); kb > 0 {
		if err := registerDuoStreamModels(srv.GetVirtualModelService(), kb, duoEnvInt(duoEnvStreamMS)); err != nil {
			return fmt.Errorf("register duo stream models: %w", err)
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
			UUID:               "tb1-openai-chat",
			Name:               "tb1-openai-chat",
			APIBase:            tb1URL + "/virtual/openai/v1",
			APIStyle:           protocol.APIStyleOpenAI,
			OpenAIEndpointMode: ai.EndpointModeChat,
		},
		"responses": {
			UUID:               "tb1-openai-responses",
			Name:               "tb1-openai-responses",
			APIBase:            tb1URL + "/virtual/openai/v1",
			APIStyle:           protocol.APIStyleOpenAI,
			OpenAIEndpointMode: ai.EndpointModeResponses,
		},
		"anthropic": {
			UUID:     "tb1-anthropic",
			Name:     "tb1-anthropic",
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
		rule := typ.Rule{
			UUID:          route.RequestModel(),
			Scenario:      typ.ScenarioAnthropic,
			RequestModel:  route.RequestModel(),
			ResponseModel: duoTargetVModel(route),
			Services: []*loadbalance.Service{{
				Provider:   providers[route.Target].UUID,
				Model:      duoTargetVModel(route),
				Weight:     1,
				Active:     true,
				TimeWindow: 300,
			}},
			LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.NewRandomParams()},
			Active:   true,
		}
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

// duoStreamChunks builds ~2 KB filler chunks totalling approximately kb KB.
func duoStreamChunks(kb int) []string {
	const chunkBytes = 2 * 1024
	n := kb * 1024 / chunkBytes
	if n < 1 {
		n = 1
	}
	const filler = "streamed backpressure payload "
	chunk := strings.Repeat(filler, chunkBytes/len(filler)+1)[:chunkBytes]
	chunks := make([]string, n)
	for i := range chunks {
		chunks[i] = chunk
	}
	return chunks
}
