package check

import (
	"encoding/json"
	"github.com/tingly-dev/tingly-box/vmodel/validate"
	"testing"
)

func TestObservationRules(t *testing.T) {
	r := &RoundTripResult{HTTPStatus: 200, RawBody: []byte(`{"id":"a","echo":"a"}`), ToolCalls: []ToolCallResult{{Name: "weather", Arguments: `{"city" : "Paris"}`}}}
	ref := "body.id"
	checks := []Assertion{AssertRule(validate.Rule{Path: "body.echo", Op: "eq", Ref: &ref}), AssertRule(validate.Rule{Path: "tool_calls.0.arguments.city", Op: "eq", Value: json.RawMessage(`"Paris"`)}), AssertRule(validate.Rule{Path: "usage", Op: "absent"})}
	if failed := Evaluate(r, checks); len(failed) != 0 {
		t.Fatal(failed)
	}
	r.RawBody = []byte(`not-json`)
	if failed := Evaluate(r, checks); len(failed) != 1 {
		t.Fatal(failed)
	}
}
