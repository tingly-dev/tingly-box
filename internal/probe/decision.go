package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/decision"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

var decisionFixture = json.RawMessage(`{"state":"My invoice has an unexpected charge.","questions":{"team":{"type":"choice","instructions":"Which team should handle this?","criteria":{"billing":"Charges and refunds","technical":"Errors and outages"}}}}`)

// buildDecisionBody is shared by execution and cURL; the selected rule owns model.
func buildDecisionBody(raw json.RawMessage, model string) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = decisionFixture
	}
	dialect, err := decision.Request(raw)
	if err != nil {
		return nil, err
	}
	root := gjson.ParseBytes(raw)
	questions := root.Get("questions")
	if dialect == decision.Unknown || !root.Get("state").Exists() && !root.Get("input").Exists() ||
		questions.IsObject() && len(questions.Map()) == 0 || questions.IsArray() && len(questions.Array()) == 0 ||
		!questions.IsObject() && !questions.IsArray() {
		return nil, fmt.Errorf("input or state and non-empty questions are required")
	}
	out, err := sjson.SetBytes(raw, "model", model)
	return json.RawMessage(out), err
}

func probeDecisions(ctx context.Context, oc client.OpenAIClientInterface, p probeParams) (*Result, error) {
	body, err := buildDecisionBody(p.DecisionRequest, p.Model)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := oc.DecisionsNew(ctx, body)
	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(resp) {
		return nil, fmt.Errorf("decision response is not valid JSON")
	}
	if err := validateDecisionResponse(body, resp); err != nil {
		return nil, err
	}
	u := gjson.GetBytes(resp, "usage")
	var usage *protocol.TokenUsage
	get := func(keys ...string) int64 {
		for _, k := range keys {
			v := u.Get(k)
			if v.Type == gjson.Number {
				return v.Int()
			}
		}
		return 0
	}
	if u.IsObject() {
		usage = protocol.NewTokenUsageWithCache(int(get("input_tokens", "inputTokens", "prompt_tokens")), int(get("output_tokens", "outputTokens", "completion_tokens")), 0)
	}
	result := toProbeResult(string(resp), time.Since(start).Milliseconds(), strings.TrimRight(oc.GetProvider().APIBase, "/")+"/decisions", false, usage, nil)
	result.DecisionResponse = json.RawMessage(resp)
	return result, nil
}

// A 200 response alone must not count as a successful decision probe: it
// needs an answer for each question in the same wire format.
func validateDecisionResponse(request, response []byte) error {
	questions := gjson.GetBytes(request, "questions")
	answers := gjson.GetBytes(response, "answers")
	check := func(q, a gjson.Result) bool {
		if !a.IsObject() {
			return false
		}
		typ := q.Get("type").String()
		if a.Get("type").String() == "refusal" {
			return true
		}
		if typ == "" || a.Get("type").String() != typ {
			return false
		}
		switch typ {
		case "noul":
			return a.Get("noul").Type == gjson.Number
		case "predicate":
			return a.Get("probability").Type == gjson.Number
		case "choice":
			return a.Get("choice").Exists()
		case "score":
			return a.Get("score").Type == gjson.Number
		default:
			return true // preserve provider-specific answer types
		}
	}
	if questions.IsObject() && answers.IsObject() {
		byName := answers.Map()
		if len(byName) != len(questions.Map()) {
			return fmt.Errorf("decision response has mismatched answers")
		}
		valid := true
		questions.ForEach(func(k, q gjson.Result) bool { valid = check(q, byName[k.String()]); return valid })
		if valid {
			return nil
		}
	}
	if questions.IsArray() && answers.IsArray() {
		qs, as := questions.Array(), answers.Array()
		if len(qs) == len(as) {
			valid := true
			for i := range qs {
				if !check(qs[i], as[i]) {
					valid = false
					break
				}
			}
			if valid {
				return nil
			}
		}
	}
	return fmt.Errorf("decision response is missing or has incompatible answers")
}
