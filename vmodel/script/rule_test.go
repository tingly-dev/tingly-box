package script

import (
	"encoding/json"
	"github.com/tingly-dev/tingly-box/vmodel/validate"
	"testing"
)

func TestSharedRules(t *testing.T) {
	m := &Match{Rules: []validate.Rule{{Path: "model", Op: "regex", Value: json.RawMessage(`"^test"`)}, {Path: "tools", Op: "contains", Value: json.RawMessage(`"weather"`)}}}
	if err := m.compile(); err != nil {
		t.Fatal(err)
	}
	r := &request{Model: "test-model", Tools: []string{"weather"}}
	if ok, why := m.eval(r, func(string) string { return "" }); !ok {
		t.Fatal(why)
	}
	r.Tools = []string{"other"}
	if ok, _ := m.eval(r, func(string) string { return "" }); ok {
		t.Fatal("missing tool accepted")
	}
	result := &ResultMatch{Rules: []validate.Rule{{Path: "is_error", Op: "eq", Value: json.RawMessage(`false`)}}}
	if ok, _ := result.eval(block{IsError: true}); ok {
		t.Fatal("error result accepted")
	}
}
