// Package projects mounts Projects (the projects/ module, see
// .design/projects.md) in Tingly Box: a Host that gives it model access
// through the gateway, and the routes behind the "projects" feature flag.
package projects

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/config"
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
}

var _ projectsapp.Host = (*Host)(nil)

func NewHost(cfg *config.Config) *Host {
	return &Host{cfg: cfg, cc: tbclient.NewTBClient(cfg)}
}

// ClientFor talks Anthropic Messages to the gateway's claude_code scenario
// with the model token; an empty model means the scenario's default tier.
func (h *Host) ClientFor(_ context.Context, model string) (llm.Client, string, error) {
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
// for, the default tier first.
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
	return out, nil
}

// ThreadEnv points each thread's Claude Code at the gateway the same way
// Desk and @cc do.
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
