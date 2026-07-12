package protocoltest

// Routing phase of the duo environment: verifies that a rule configured with
// smart routing behaves end-to-end — the rule is created through tb2's
// production rule API, requests with controlled shapes are driven over real
// HTTP, and each decision is asserted on TWO independent surfaces:
//
//   - wire level: which service answered, read from the response body (each
//     tb1 service-identity vmodel replies with its own marker, see
//     DuoServiceMarker) — no cooperation from the gateway required;
//   - explanation level: tb2's /api/v1/requests/:id timeline, which joins
//     the smart_routing evaluation trace (outcome, matched rule, selected
//     service) by the parent-supplied X-Request-Id — the same surface a
//     user debugging their routing config reads.
//
// Scenarios are declarative (rule shape + request shapes + expectations) so
// built-ins and user-supplied YAML files run through one engine. See
// duo_routing_scenarios.go for the built-in catalog.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ─── Scenario model ───────────────────────────────────────────────────────────

// DuoRoutingService names one upstream candidate: a service identity from
// tb1's pool (DuoServiceIdentities) reached through one of tb2's provider
// protocols.
type DuoRoutingService struct {
	// Svc is the service identity ("a".."f"); the tb1 vmodel is
	// DuoServiceModel(Svc) and its response carries DuoServiceMarker(Svc).
	Svc string `yaml:"svc" json:"svc"`
	// Target is the provider protocol: "chat" (default), "responses", or
	// "anthropic".
	Target string `yaml:"target,omitempty" json:"target,omitempty"`
}

// DuoSmartOpSpec is one smart-routing condition in scenario form; it maps
// 1:1 onto smartrouting.SmartOp.
type DuoSmartOpSpec struct {
	Position  string `yaml:"position" json:"position"`
	Operation string `yaml:"operation" json:"operation"`
	Value     string `yaml:"value,omitempty" json:"value,omitempty"`
}

// DuoSmartPartition is one smart-routing rule: AND-ed ops selecting a
// service subset. First matching partition wins.
type DuoSmartPartition struct {
	Description string              `yaml:"description" json:"description"`
	Ops         []DuoSmartOpSpec    `yaml:"ops" json:"ops"`
	Services    []DuoRoutingService `yaml:"services" json:"services"`
}

// DuoRoutingRule is the rule shape under test.
type DuoRoutingRule struct {
	// Scenario is the gateway scenario: "anthropic" (default) or
	// "claude_code" (required for the agent.claude_code position).
	Scenario string `yaml:"scenario,omitempty" json:"scenario,omitempty"`
	// AffinitySecs enables session affinity (Flags.SessionAffinity).
	AffinitySecs int `yaml:"affinity_secs,omitempty" json:"affinity_secs,omitempty"`
	// Services is the base pool the LB falls back to when no partition
	// matches.
	Services []DuoRoutingService `yaml:"services" json:"services"`
	// Smart lists the partitions, evaluated in order.
	Smart []DuoSmartPartition `yaml:"smart" json:"smart"`
}

// DuoRoutingBody describes the request shape (built as an Anthropic
// messages request; valid for both v1 and beta surfaces).
type DuoRoutingBody struct {
	// SizeKB pads the conversation with filler user text (drives the token
	// position: tokens ≈ SizeKB*1024/4).
	SizeKB int `yaml:"size_kb,omitempty" json:"size_kb,omitempty"`
	// UserText is the final user message ("duo routing probe" if empty).
	UserText string `yaml:"user_text,omitempty" json:"user_text,omitempty"`
	// System sets the system prompt (e.g. Claude Code fingerprints).
	System string `yaml:"system,omitempty" json:"system,omitempty"`
	// Thinking enables the thinking parameter.
	Thinking bool `yaml:"thinking,omitempty" json:"thinking,omitempty"`
}

// DuoRoutingExpect is the per-request expectation; empty fields are skipped.
type DuoRoutingExpect struct {
	// Svc asserts which service identity answered (wire level).
	Svc string `yaml:"svc,omitempty" json:"svc,omitempty"`
	// Outcome asserts the smart-routing trace outcome ("matched",
	// "no_match", ...).
	Outcome string `yaml:"outcome,omitempty" json:"outcome,omitempty"`
	// Matched asserts the matched partition's description.
	Matched string `yaml:"matched,omitempty" json:"matched,omitempty"`
}

// DuoRoutingRequest is one request in a scenario's program.
type DuoRoutingRequest struct {
	Name string `yaml:"name" json:"name"`
	// Beta selects the Anthropic beta surface (?beta=true).
	Beta bool `yaml:"beta,omitempty" json:"beta,omitempty"`
	// Session sets X-Tingly-Session-ID (affinity identity); "" = none.
	Session string           `yaml:"session,omitempty" json:"session,omitempty"`
	Body    DuoRoutingBody   `yaml:"body" json:"body"`
	Expect  DuoRoutingExpect `yaml:"expect" json:"expect"`
}

// DuoRoutingScenario is one rule shape plus a request program against it.
type DuoRoutingScenario struct {
	Name        string              `yaml:"name" json:"name"`
	Description string              `yaml:"description,omitempty" json:"description,omitempty"`
	Rule        DuoRoutingRule      `yaml:"rule" json:"rule"`
	Requests    []DuoRoutingRequest `yaml:"requests" json:"requests"`
}

// RequestModel returns the tb2 request model the scenario's rule binds.
func (sc *DuoRoutingScenario) RequestModel() string { return "duo-route-" + sc.Name }

func (sc *DuoRoutingScenario) scenario() string {
	if sc.Rule.Scenario != "" {
		return sc.Rule.Scenario
	}
	return string(typ.ScenarioAnthropic)
}

// ─── Rule construction ────────────────────────────────────────────────────────

func duoRoutingProviderUUID(target string) (string, error) {
	switch target {
	case "", "chat":
		return DuoProviderChat, nil
	case "responses":
		return DuoProviderResponses, nil
	case "anthropic":
		return DuoProviderAnthropic, nil
	default:
		return "", fmt.Errorf("unknown service target %q (chat|responses|anthropic)", target)
	}
}

func duoRoutingServices(specs []DuoRoutingService) ([]*loadbalance.Service, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one service required")
	}
	services := make([]*loadbalance.Service, 0, len(specs))
	for _, s := range specs {
		provider, err := duoRoutingProviderUUID(s.Target)
		if err != nil {
			return nil, err
		}
		services = append(services, harnessService(provider, DuoServiceModel(s.Svc)))
	}
	return services, nil
}

// toRule maps the scenario onto the exact typ.Rule JSON a user would POST.
func (sc *DuoRoutingScenario) toRule() (*typ.Rule, error) {
	base, err := duoRoutingServices(sc.Rule.Services)
	if err != nil {
		return nil, fmt.Errorf("scenario %s base services: %w", sc.Name, err)
	}
	smart := make([]smartrouting.SmartRouting, 0, len(sc.Rule.Smart))
	for _, p := range sc.Rule.Smart {
		services, err := duoRoutingServices(p.Services)
		if err != nil {
			return nil, fmt.Errorf("scenario %s partition %q: %w", sc.Name, p.Description, err)
		}
		ops := make([]smartrouting.SmartOp, 0, len(p.Ops))
		for _, op := range p.Ops {
			ops = append(ops, smartrouting.SmartOp{
				Position:  smartrouting.SmartOpPosition(op.Position),
				Operation: smartrouting.SmartOpOperation(op.Operation),
				Value:     op.Value,
			})
		}
		smart = append(smart, smartrouting.SmartRouting{
			Description: p.Description,
			Ops:         ops,
			Services:    services,
		})
	}
	// UUID "" — the rule enters through tb2's production API, which assigns one.
	rule := newHarnessRule("", typ.RuleScenario(sc.scenario()), sc.RequestModel(), base[0].Model, base...)
	rule.SmartEnabled = len(smart) > 0
	rule.SmartRouting = smart
	rule.Flags.SessionAffinity = sc.Rule.AffinitySecs
	return &rule, nil
}

// seedRoutingRule creates the scenario's rule on tb2 through the production
// rule API — the same path a user's configuration takes.
func (env *DuoEnv) seedRoutingRule(sc *DuoRoutingScenario) error {
	rule, err := sc.toRule()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, env.TB2.BaseURL+"/api/v1/rule", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.TB2.UserToken)
	resp, err := env.client.Do(req)
	if err != nil {
		return fmt.Errorf("create rule %s: %w", sc.RequestModel(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("create rule %s: status %d: %s", sc.RequestModel(), resp.StatusCode, b)
	}
	return nil
}

// ─── Request building ─────────────────────────────────────────────────────────

// buildRoutingBody constructs the Anthropic messages request for one
// scenario request.
func buildRoutingBody(sc *DuoRoutingScenario, r DuoRoutingRequest) ([]byte, error) {
	userText := r.Body.UserText
	if userText == "" {
		userText = "duo routing probe"
	}

	var messages []map[string]any
	if r.Body.SizeKB > 0 {
		pad := duoFiller(r.Body.SizeKB * 1024)
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": pad}},
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": []map[string]any{{"type": "text", "text": userText}},
	})

	body := map[string]any{
		"model":      sc.RequestModel(),
		"max_tokens": 2048,
		"stream":     false,
		"messages":   messages,
	}
	if r.Body.System != "" {
		// Block form, not a bare string — the beta binding drops a string
		// system, and Claude Code itself sends the array form.
		body["system"] = []map[string]any{{"type": "text", "text": r.Body.System}}
	}
	if r.Body.Thinking {
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 1024}
	}
	return json.Marshal(body)
}

// ─── Trace fetching ───────────────────────────────────────────────────────────

// duoRequestDetail mirrors the /api/v1/requests/:id response shape the
// engine consumes.
type duoRequestDetail struct {
	RequestID   string `json:"request_id"`
	RoutedModel string `json:"routed_model"`
	Events      []struct {
		Source string         `json:"source"`
		Fields map[string]any `json:"fields"`
	} `json:"events"`
}

// smartRoutingTrace fetches the request's timeline from the instance and
// returns the smart_routing event's fields plus the summary's routed_model
// (the model the LB ultimately dispatched, folded from the access log).
// Sink writes race the response, so it retries briefly.
func (inst *DuoInstance) smartRoutingTrace(requestID string) (fields map[string]any, routedModel string, err error) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		fields, routedModel, err = inst.smartRoutingTraceOnce(requestID)
		if err == nil && routedModel != "" {
			return fields, routedModel, nil
		}
		if time.Now().After(deadline) {
			if err == nil {
				err = fmt.Errorf("request %s: routed_model not recorded", requestID)
			}
			return nil, "", err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (inst *DuoInstance) smartRoutingTraceOnce(requestID string) (map[string]any, string, error) {
	var detail duoRequestDetail
	if err := inst.getJSON("/api/v1/requests/"+requestID, &detail); err != nil {
		return nil, "", err
	}
	for _, ev := range detail.Events {
		if ev.Source == "smart_routing" {
			return ev.Fields, detail.RoutedModel, nil
		}
	}
	return nil, "", fmt.Errorf("request %s: no smart_routing event in timeline (%d events)", requestID, len(detail.Events))
}

// ─── Scenario execution ───────────────────────────────────────────────────────

// RunRoutingScenario seeds the scenario's rule and drives its request
// program, returning one DuoCheck per assertion.
func (env *DuoEnv) RunRoutingScenario(sc *DuoRoutingScenario) []DuoCheck {
	var checks []DuoCheck
	add := func(request, name string, pass bool, detail string) {
		checks = append(checks, DuoCheck{
			Route:  sc.Name,
			Name:   request + "/" + name,
			Pass:   pass,
			Detail: detail,
		})
	}

	if err := env.seedRoutingRule(sc); err != nil {
		add("rule", "create", false, err.Error())
		return checks
	}
	add("rule", "create", true, sc.RequestModel())

	for i, r := range sc.Requests {
		requestID := fmt.Sprintf("duo-route-%s-%02d-%s", sc.Name, i, r.Name)
		env.runRoutingRequest(sc, r, requestID, func(name string, pass bool, detail string) {
			add(r.Name, name, pass, detail)
		})
	}
	return checks
}

func (env *DuoEnv) runRoutingRequest(sc *DuoRoutingScenario, r DuoRoutingRequest, requestID string, add func(name string, pass bool, detail string)) {
	payload, err := buildRoutingBody(sc, r)
	if err != nil {
		add("body", false, err.Error())
		return
	}

	path := "/tingly/" + sc.scenario() + "/v1/messages"
	if r.Beta {
		path += "?beta=true"
	}
	req, err := http.NewRequest(http.MethodPost, env.TB2.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		add("http", false, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.TB2.ModelToken)
	req.Header.Set("X-Request-Id", requestID)
	if r.Session != "" {
		req.Header.Set("X-Tingly-Session-ID", r.Session)
	}
	resp, err := env.client.Do(req)
	if err != nil {
		add("http", false, err.Error())
		return
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		add("http", false, fmt.Sprintf("status %d: %s", resp.StatusCode, raw[:min(len(raw), 1024)]))
		return
	}
	add("http", true, "200")

	// Wire-level: which service identity answered.
	if r.Expect.Svc != "" {
		var m map[string]any
		content := ""
		if err := json.Unmarshal(raw, &m); err == nil {
			if parsed := sse.ParseAnthropicResult(m); parsed != nil {
				content = parsed.Content
			}
		}
		marker := DuoServiceMarker(r.Expect.Svc)
		add("wire/svc", strings.Contains(content, marker),
			fmt.Sprintf("want %s, got %q", marker, content[:min(len(content), 120)]))
	}

	// Explanation-level: the smart_routing trace joined by request id.
	if r.Expect.Outcome == "" && r.Expect.Matched == "" && r.Expect.Svc == "" {
		return
	}
	fields, routedModel, err := env.TB2.smartRoutingTrace(requestID)
	if err != nil {
		add("trace", false, err.Error())
		return
	}
	if r.Expect.Outcome != "" {
		got, _ := fields["outcome"].(string)
		reason, _ := fields["reason"].(string)
		add("trace/outcome", got == r.Expect.Outcome, fmt.Sprintf("want %s, got %s (%s)", r.Expect.Outcome, got, reason))
	}
	if r.Expect.Matched != "" {
		got, _ := fields["matched_rule_description"].(string)
		add("trace/matched", got == r.Expect.Matched, fmt.Sprintf("want %q, got %q", r.Expect.Matched, got))
	}
	if r.Expect.Svc != "" {
		want := DuoServiceModel(r.Expect.Svc)
		add("trace/routed_model", routedModel == want, fmt.Sprintf("want %s, got %s", want, routedModel))
	}
}
