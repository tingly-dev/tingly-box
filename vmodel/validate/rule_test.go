package validate

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRuleMatch(t *testing.T) {
	raw := []byte(`{"id":"a","echo":"a","value":null,"items":["x",2],"count":9007199254740993,"body":{"text":"hello"}}`)
	ref := "id"
	for _, tc := range []struct {
		rule Rule
		pass bool
	}{
		{Rule{Path: "echo", Op: "eq", Ref: &ref}, true},
		{Rule{Path: "value", Op: "exists"}, true},
		{Rule{Path: "value", Op: "eq", Value: json.RawMessage(`null`)}, true},
		{Rule{Path: "missing", Op: "absent"}, true},
		{Rule{Path: "missing", Op: "ne", Value: json.RawMessage(`null`)}, false},
		{Rule{Path: "items", Op: "contains", Value: json.RawMessage(`2`)}, true},
		{Rule{Path: "items", Op: "contains", Value: json.RawMessage(`"2"`)}, false},
		{Rule{Path: "body.text", Op: "regex", Value: json.RawMessage(`"^h"`)}, true},
		{Rule{Path: "count", Op: "gte", Value: json.RawMessage(`9007199254740994`)}, false},
		{Rule{Path: "id", Op: "one_of", Value: json.RawMessage(`["b","a"]`)}, true},
		{Rule{Path: "id", Op: "regex", Value: json.RawMessage(`"["`)}, false},
		{Rule{Path: "id", Op: "eq", Value: json.RawMessage(`"a"`), Ref: &ref}, false},
	} {
		if err := tc.rule.Match(raw); (err == nil) != tc.pass {
			t.Errorf("%+v: got %v, want pass=%v", tc.rule, err, tc.pass)
		}
	}
	if err := EqualJSON([]byte(`{"b":2,"a":1}`), []byte(`{"a":1,"b":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := EqualJSON([]byte(`[1,2]`), []byte(`[2,1]`)); err == nil {
		t.Fatal("array order ignored")
	}
}

func TestEvaluateKeepsFailures(t *testing.T) {
	checks := []NamedCheck[int]{{"first", func(int) error { return errors.New("first failed") }}, {"pass", func(int) error { return nil }}, {"last", func(int) error { return errors.New("last failed") }}}
	failed := Evaluate(2, checks)
	if len(failed) != 2 || failed[0].Name != "first" || failed[1].Name != "last" {
		t.Fatalf("%v", failed)
	}
}
