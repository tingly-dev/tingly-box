package protocoltest

import (
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/thinking"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// This file is the thinking × output-limit combination suite, shared by the go
// test entry point (TestThinkingLimits) and the CLI
// (`harness matrix --mode=thinking_limits`).
//
// Thinking effort and output limits are each set in more than one place — the
// client's own request, the rule's thinking_effort flag, the model's output
// limit — and each is applied at a different step of the pipeline
// (.design/protocol-stage-pipeline.md). A single case per knob cannot show how
// they combine, so this suite crosses them:
//
//	source × target × client thinking level × client max_tokens
//	       × rule thinking_effort × rule max-tokens field flag (Chat targets)
//	       × streaming
//
// and checks, on the request actually forwarded upstream, the properties every
// combination must keep:
//
//   - the output-token field(s) are within the model's limit, and on a Chat
//     target the field is the one use_max_completion_tokens / use_max_tokens
//     asks for;
//   - on an Anthropic target, a thinking budget is >= 1024 and strictly below
//     max_tokens (Anthropic rejects anything else);
//   - the effort that reaches the provider is the effective one: the rule's
//     level when set, none when the rule says off, else the client's level —
//     collapsed through the generic tier map on an OpenAI Chat target whenever
//     the gateway derived it.
//
// The test model is not in the catalog, so its output limit is the 8192
// fallback, and the test provider is not api.openai.com, so Chat targets get
// the generic effort tiers. Google targets are out of scope here.

const (
	thinkingLimitsModelLimit = 8192
	thinkingLimitsScenario   = "thinking_limits"
)

// thinkingLimitsClientLevels are the client thinking levels crossed: none plus
// the full canonical ladder. An Anthropic client expresses a level as the
// ladder's budget_tokens (thinking.BudgetMapping), an OpenAI client as the
// effort string itself.
var thinkingLimitsClientLevels = []string{
	"", thinking.LevelMinimal, thinking.LevelLow, thinking.LevelMedium,
	thinking.LevelHigh, thinking.LevelXHigh, thinking.LevelMax,
}

// thinkingLimitsRuleLevels are the rule thinking_effort values the UI offers.
var thinkingLimitsRuleLevels = []string{
	typ.ThinkingEffortDefault, typ.ThinkingEffortOff, typ.ThinkingEffortLow,
	typ.ThinkingEffortMedium, typ.ThinkingEffortHigh, typ.ThinkingEffortMax,
}

var thinkingLimitsSources = []protocol.APIType{
	protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta,
	protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses,
}

var thinkingLimitsTargets = []protocol.APIType{
	protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses,
}

// thinkingLimitsMaxTokens are the client limits crossed: below every budget
// but the smallest, and above the model limit. OpenAI clients may also omit
// the field (0); Anthropic requires it.
func thinkingLimitsMaxTokens(source protocol.APIType) []int64 {
	if isAnthropicAPI(source) {
		return []int64{2048, 40000}
	}
	return []int64{0, 2048, 40000}
}

func isAnthropicAPI(api protocol.APIType) bool {
	return api == protocol.TypeAnthropicV1 || api == protocol.TypeAnthropicBeta
}

// Rule flags that pick the Chat limit field (Type 1b-post, applied after the
// output limit).
const (
	tokenFieldAsSent              = ""
	tokenFieldMaxCompletionTokens = "use_max_completion_tokens"
	tokenFieldMaxTokens           = "use_max_tokens"
)

// thinkingLimitsTokenFlags are the field flags crossed for a target: all three
// on OpenAI Chat, none elsewhere (they only act on the Chat shape).
func thinkingLimitsTokenFlags(target protocol.APIType) []string {
	if target == protocol.TypeOpenAIChat {
		return []string{tokenFieldAsSent, tokenFieldMaxCompletionTokens, tokenFieldMaxTokens}
	}
	return []string{tokenFieldAsSent}
}

// thinkingLimitsGroup is one route: every combination sharing (source,
// target, rule flags) runs against one env with that rule configured.
type thinkingLimitsGroup struct {
	source, target protocol.APIType
	rule           string
	tokenFlag      string
}

func (g thinkingLimitsGroup) ruleFlags() typ.RuleFlags {
	return typ.RuleFlags{
		ThinkingEffort:         g.rule,
		UseMaxCompletionTokens: g.tokenFlag == tokenFieldMaxCompletionTokens,
		UseMaxTokens:           g.tokenFlag == tokenFieldMaxTokens,
	}
}

type thinkingLimitsCase struct {
	thinkingLimitsGroup
	client    string
	maxTokens int64
	streaming bool
}

func (c thinkingLimitsCase) name() string {
	client, rule, maxTokens := c.client, c.rule, fmt.Sprint(c.maxTokens)
	if client == "" {
		client = "none"
	}
	if rule == "" {
		rule = "client"
	}
	if c.maxTokens == 0 {
		maxTokens = "absent"
	}
	mode := "nonstream"
	if c.streaming {
		mode = "stream"
	}
	if c.tokenFlag != tokenFieldAsSent {
		rule += "+" + c.tokenFlag
	}
	return fmt.Sprintf("thinking_limits/%s->%s/client=%s/max_tokens=%s/rule=%s/%s",
		c.source, c.target, client, maxTokens, rule, mode)
}

func thinkingLimitsGroups() []thinkingLimitsGroup {
	var groups []thinkingLimitsGroup
	for _, source := range thinkingLimitsSources {
		for _, target := range thinkingLimitsTargets {
			for _, rule := range thinkingLimitsRuleLevels {
				for _, tokenFlag := range thinkingLimitsTokenFlags(target) {
					groups = append(groups, thinkingLimitsGroup{source: source, target: target, rule: rule, tokenFlag: tokenFlag})
				}
			}
		}
	}
	return groups
}

func (g thinkingLimitsGroup) cases() []thinkingLimitsCase {
	var cases []thinkingLimitsCase
	for _, client := range thinkingLimitsClientLevels {
		for _, maxTokens := range thinkingLimitsMaxTokens(g.source) {
			for _, streaming := range []bool{false, true} {
				cases = append(cases, thinkingLimitsCase{thinkingLimitsGroup: g, client: client, maxTokens: maxTokens, streaming: streaming})
			}
		}
	}
	return cases
}

// thinkingLimitsBody builds the client request for one combination.
func thinkingLimitsBody(c thinkingLimitsCase, model string) (string, []byte) {
	path, _ := buildRequest(c.source, model, c.streaming) // only the path is reused
	body := map[string]any{"model": model, "stream": c.streaming}
	switch c.source {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		body["max_tokens"] = c.maxTokens
		body["messages"] = []map[string]any{{"role": "user", "content": "Hello"}}
		if c.client != "" {
			body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": thinking.BudgetMapping[c.client]}
		}
	case protocol.TypeOpenAIChat:
		body["messages"] = []map[string]any{{"role": "user", "content": "Hello"}}
		if c.maxTokens > 0 {
			body["max_tokens"] = c.maxTokens
		}
		if c.client != "" {
			body["reasoning_effort"] = c.client
		}
	case protocol.TypeOpenAIResponses:
		body["input"] = "Hello"
		if c.maxTokens > 0 {
			body["max_output_tokens"] = c.maxTokens
		}
		if c.client != "" {
			body["reasoning"] = map[string]any{"effort": c.client}
		}
	}
	return path, mustMarshal(body)
}

// effectiveLevel is the thinking level the provider should get: the rule's
// when set, none when the rule turns thinking off, else the client's.
func (c thinkingLimitsCase) effectiveLevel() string {
	switch c.rule {
	case typ.ThinkingEffortDefault:
		return c.client
	case typ.ThinkingEffortOff:
		return ""
	}
	return c.rule
}

// genericEffortTier mirrors ops.genericEffortTiers: the reasoning_effort an
// unverified OpenAI-compatible host gets for a ladder level.
func genericEffortTier(level string) string {
	switch level {
	case thinking.LevelMinimal, thinking.LevelLow:
		return "low"
	case thinking.LevelMedium:
		return "medium"
	case thinking.LevelHigh, thinking.LevelXHigh, thinking.LevelMax:
		return "high"
	}
	return level
}

func targetEndpoint(target protocol.APIType) EndpointKind {
	switch target {
	case protocol.TypeOpenAIChat:
		return EndpointChat
	case protocol.TypeOpenAIResponses:
		return EndpointResponses
	}
	return EndpointAnthropic
}

func numberField(body map[string]any, path ...string) (float64, bool) {
	var v any = body
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return 0, false
		}
		v = m[key]
	}
	n, ok := v.(float64)
	return n, ok
}

func stringField(body map[string]any, path ...string) string {
	var v any = body
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[key]
	}
	s, _ := v.(string)
	return s
}

// checkThinkingLimits sends one combination and returns every violated
// property of the request that reached the provider.
func checkThinkingLimits(env *TestEnv, c thinkingLimitsCase, model string) []string {
	endpoint := targetEndpoint(c.target)
	hitsBefore := env.virtual.EndpointHits(endpoint)
	path, body := thinkingLimitsBody(c, model)
	res, err := env.dispatch(c.source, c.target, thinkingLimitsScenario, path, body, nil, c.streaming)
	if err != nil {
		return []string{fmt.Sprintf("dispatch: %v", err)}
	}
	if res.HTTPStatus != 200 {
		return []string{fmt.Sprintf("client got HTTP %d: %s", res.HTTPStatus, truncate(string(res.RawBody), 200))}
	}
	if env.virtual.EndpointHits(endpoint) != hitsBefore+1 {
		return []string{fmt.Sprintf("expected one upstream %s request, got %d", endpoint, env.virtual.EndpointHits(endpoint)-hitsBefore)}
	}
	up := env.virtual.LastRequest(endpoint).JSON()

	var failures []string
	limit := float64(thinkingLimitsModelLimit)
	checkLimit := func(field string) {
		if n, ok := numberField(up, field); ok && n > limit {
			failures = append(failures, fmt.Sprintf("upstream %s = %.0f exceeds the model limit %.0f", field, n, limit))
		}
	}
	want := c.effectiveLevel()

	switch c.target {
	case protocol.TypeOpenAIChat:
		checkLimit("max_tokens")
		checkLimit("max_completion_tokens")
		_, hasMaxTokens := numberField(up, "max_tokens")
		_, hasMaxCompletion := numberField(up, "max_completion_tokens")
		switch {
		case c.tokenFlag == tokenFieldMaxCompletionTokens && hasMaxTokens:
			failures = append(failures, "use_max_completion_tokens: upstream still carries max_tokens")
		case c.tokenFlag == tokenFieldMaxTokens && hasMaxCompletion:
			failures = append(failures, "use_max_tokens: upstream still carries max_completion_tokens")
		}
		wantEffort := genericEffortTier(want)
		if c.source == protocol.TypeOpenAIChat && c.rule == typ.ThinkingEffortDefault {
			// A Chat client's own reasoning_effort is forwarded verbatim
			// (pipeline doc, deviation 3).
			wantEffort = want
		}
		if got := stringField(up, "reasoning_effort"); got != wantEffort {
			failures = append(failures, fmt.Sprintf("upstream reasoning_effort = %q, want %q", got, wantEffort))
		}

	case protocol.TypeOpenAIResponses:
		checkLimit("max_output_tokens")
		got := stringField(up, "reasoning", "effort")
		if got == "none" {
			got = ""
		}
		if got != want {
			failures = append(failures, fmt.Sprintf("upstream reasoning.effort = %q, want %q", got, want))
		}

	default: // Anthropic
		checkLimit("max_tokens")
		maxTokens, _ := numberField(up, "max_tokens")
		thinkingType := stringField(up, "thinking", "type")
		switch {
		case want == "" && thinkingType != "" && thinkingType != "disabled":
			failures = append(failures, fmt.Sprintf("upstream thinking.type = %q, want off (absent or disabled)", thinkingType))
		case want != "" && thinkingType != "enabled" && thinkingType != "adaptive":
			failures = append(failures, fmt.Sprintf("upstream thinking.type = %q, want thinking on at %q", thinkingType, want))
		}
		if budget, ok := numberField(up, "thinking", "budget_tokens"); ok && thinkingType == "enabled" {
			if budget < 1024 || budget >= maxTokens {
				failures = append(failures, fmt.Sprintf("upstream budget_tokens = %.0f with max_tokens = %.0f; Anthropic needs 1024 <= budget < max_tokens", budget, maxTokens))
			}
		}
	}
	return failures
}

// runThinkingLimitsGroup runs every combination of one route and returns one
// result per combination, judged against the known-gap registry.
func runThinkingLimitsGroup(g thinkingLimitsGroup) []TestResult {
	cases := g.cases()
	results := make([]TestResult, len(cases))
	env, err := NewTestEnvForCLI()
	if err != nil {
		for i, c := range cases {
			results[i] = setupFailureResult(TestResult{Name: c.name(), Scenario: thinkingLimitsScenario, Source: c.source, Target: c.target, Streaming: c.streaming}, err)
		}
		return results
	}
	defer env.Close()

	s := TextScenario()
	s.Name = thinkingLimitsScenario
	s.Assertions = nil
	model := env.SetupRouteWithFlags(g.source, g.target, s, g.ruleFlags())

	for i, c := range cases {
		start := time.Now()
		res := TestResult{Name: c.name(), Scenario: thinkingLimitsScenario, Source: c.source, Target: c.target, Streaming: c.streaming}
		failures := checkThinkingLimits(env, c, model)
		errs, gap := judgeCase(c.name(), failures)
		switch {
		case gap != nil:
			res.KnownGap = gap.ID
			for _, f := range failures {
				res.Errors = append(res.Errors, AssertionError{Assertion: "known gap " + gap.ID, Error: f, Context: gap.Reason})
			}
		default:
			for _, e := range errs {
				res.Errors = append(res.Errors, AssertionError{Assertion: "thinking_limits", Error: e})
			}
		}
		res.Passed = len(failures) == 0 && len(errs) == 0
		res.Duration = time.Since(start)
		results[i] = res
	}
	return results
}

// ExecuteAllThinkingLimits runs the combination suite without testing.T. It
// is the CLI counterpart of TestThinkingLimits.
func (m *Matrix) ExecuteAllThinkingLimits() []TestResult {
	groups := thinkingLimitsGroups()
	perGroup := make([][]TestResult, len(groups))
	runIndexed(len(groups), m.sectionParallelism(), func(i int) {
		perGroup[i] = runThinkingLimitsGroup(groups[i])
	})
	var results []TestResult
	for _, r := range perGroup {
		results = append(results, r...)
	}
	return results
}

// thinkingLimitsFailureSummary renders a result's errors on one line.
func thinkingLimitsFailureSummary(r TestResult) string {
	var parts []string
	for _, e := range r.Errors {
		parts = append(parts, e.Error)
	}
	return strings.Join(parts, "; ")
}
