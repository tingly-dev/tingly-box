// Package projects mounts Projects (the projects/ module, see
// .design/projects.md) in Tingly Box: a Host that gives it model access
// through the gateway, and the routes behind the "projects" feature flag.
package projects

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// Host gives Projects model access through Tingly Box's own gateway. The
// coordinator and the threads both go through the claude_code scenario, so
// its routing rules, usage records and quotas cover Projects too, and the
// model names Projects offers are the ones that scenario routes.
type Host struct {
	cfg *config.Config
	cc  *tbclient.TBClientImpl
	// routes resolves each listed model to the real model behind it via the
	// status line's own PreviewRoute; nil leaves the list as the bare ids.
	routes *statusline.Handler
	// bridge carries Projects messages to IM chats; see chatbridge.go.
	bridge *ChatBridge
}

var _ projectsapp.Host = (*Host)(nil)

func NewHost(cfg *config.Config) *Host {
	return &Host{cfg: cfg, cc: tbclient.NewTBClient(cfg)}
}

// WithRoutePreview names the status line's route preview, so the model list
// can show what each id resolves to. Same wiring as the scenario module's
// tier listing (server_webui_api.go).
func (h *Host) WithRoutePreview(routes *statusline.Handler) *Host {
	h.routes = routes
	return h
}

// ClientFor talks Anthropic Messages to the gateway's claude_code scenario
// with the model token; an empty model means the scenario's default tier.
func (h *Host) ClientFor(ctx context.Context, model string) (llm.Client, string, error) {
	for _, alias := range agent.ClaudeCodeTierAliases {
		if model == alias && h.CheckLaunch(ctx, "", model) == nil {
			return h.ClientForProfile(ctx, "", model)
		}
	}
	if model == "" {
		model = h.cc.ClaudeCodeDefaultModel()
	}
	c, err := llm.New(llm.Connection{
		APIStyle: llm.StyleAnthropic,
		BaseURL:  h.gatewayURL() + tbclient.GetScenarioEndpointPath(typ.ScenarioClaudeCode),
		APIKey:   h.cfg.GetModelToken(),
	})
	return c, model, err
}

// Models lists the request models the claude_code scenario has active rules
// for, the default tier first. Each entry also carries what the status line
// would route it to, so an alias can be shown with the real model it stands
// for.
func (h *Host) Models(context.Context) ([]llm.Model, error) {
	def := h.cc.ClaudeCodeDefaultModel()
	out := []llm.Model{{ID: def, DisplayName: def}}
	seen := map[string]bool{def: true}
	for _, rule := range h.cfg.GetRequestConfigs() {
		m := strings.TrimSpace(rule.RequestModel)
		if !rule.Active || rule.GetScenario() != typ.ScenarioClaudeCode || m == "" || seen[m] {
			continue
		}
		seen[m] = true
		name := m
		if d := strings.TrimSpace(rule.Description); d != "" {
			name = d
		}
		out = append(out, llm.Model{ID: m, DisplayName: name})
	}
	if h.routes != nil {
		for i := range out {
			if route := h.routes.PreviewRoute(string(typ.ScenarioClaudeCode), out[i].ID); route != nil {
				out[i].ProviderModel = route.Model
				out[i].ProviderName = route.ProviderName
			}
		}
	}
	return out, nil
}

// ThreadEnv points each thread's Claude Code at the gateway the same way
// @cc does.
func (h *Host) ThreadEnv(ctx context.Context) ([]string, string) {
	env, err := h.cc.GetClaudeCodeEnv(ctx)
	if err != nil {
		return nil, ""
	}
	return env, h.cc.ClaudeCodeDefaultModel()
}

func (h *Host) gatewayURL() string {
	port := h.cfg.GetServerPort()
	if port == 0 {
		port = 12580
	}
	return fmt.Sprintf("http://%s:%d", h.cfg.GetServerHost(), port)
}

// ClientForProfile resolves the same profile and tier that Claude Code uses.
func (h *Host) ClientForProfile(ctx context.Context, profile, model string) (llm.Client, string, error) {
	env, err := h.claudeEnv(ctx, profile)
	if err != nil {
		return nil, "", err
	}
	tiers := agent.ClaudeCodeTiersFromEnv(env)
	selected := ""
	for _, tier := range tiers.Tiers {
		if tier.Alias == model {
			selected = tier.Model
			break
		}
	}
	if selected == "" {
		return nil, "", fmt.Errorf("model %q is unavailable in profile %q", model, profile)
	}
	token := env["ANTHROPIC_AUTH_TOKEN"]
	if token == "" {
		token = env["ANTHROPIC_API_KEY"]
	}
	base := env["ANTHROPIC_BASE_URL"]
	if base == "" || token == "" {
		return nil, "", fmt.Errorf("profile %q has incomplete routing", profile)
	}
	c, err := llm.New(llm.Connection{APIStyle: llm.StyleAnthropic, BaseURL: base, APIKey: token})
	return c, selected, err
}
