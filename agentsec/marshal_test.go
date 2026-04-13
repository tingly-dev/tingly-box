package agentsec

import (
	"encoding/json"
	"testing"
)

// TestRulesSlice_MarshalJSON tests the JSON marshaling of RulesSlice.
func TestRulesSlice_MarshalJSON(t *testing.T) {
	rules := RulesSlice{
		ExactRule{Tool: "Bash", Input: "pwd"},
		PrefixRule{Tool: "Bash", Prefix: "git"},
		AnyToolRule{Tool: "Read"},
	}

	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Should be a JSON array of strings
	var got []string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal to []string failed: %v", err)
	}

	expected := []string{"Bash(pwd)", "Bash(git *)", "Read"}
	if len(got) != len(expected) {
		t.Fatalf("Length mismatch: got %d, want %d", len(got), len(expected))
	}

	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("Element %d: got %q, want %q", i, got[i], expected[i])
		}
	}
}

// TestRulesSlice_UnmarshalJSON tests the JSON unmarshaling of RulesSlice.
func TestRulesSlice_UnmarshalJSON(t *testing.T) {
	input := `["Bash(pwd)", "Bash(git *)", "Read"]`

	var rules RulesSlice
	if err := json.Unmarshal([]byte(input), &rules); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if len(rules) != 3 {
		t.Fatalf("Length: got %d, want 3", len(rules))
	}

	// Check first rule (ExactRule)
	if exact, ok := rules[0].(ExactRule); ok {
		if exact.Tool != "Bash" || exact.Input != "pwd" {
			t.Errorf("First rule: got %+v, want {Tool: Bash, Input: pwd}", exact)
		}
	} else {
		t.Errorf("First rule type: got %T, want ExactRule", rules[0])
	}

	// Check second rule (PrefixRule)
	if prefix, ok := rules[1].(PrefixRule); ok {
		if prefix.Tool != "Bash" || prefix.Prefix != "git" {
			t.Errorf("Second rule: got %+v, want {Tool: Bash, Prefix: git}", prefix)
		}
	} else {
		t.Errorf("Second rule type: got %T, want PrefixRule", rules[1])
	}

	// Check third rule (AnyToolRule)
	if any, ok := rules[2].(AnyToolRule); ok {
		if any.Tool != "Read" {
			t.Errorf("Third rule: got %+v, want {Tool: Read}", any)
		}
	} else {
		t.Errorf("Third rule type: got %T, want AnyToolRule", rules[2])
	}
}

// TestRulesSlice_RoundTrip tests that marshal then unmarshal produces equivalent rules.
func TestRulesSlice_RoundTrip(t *testing.T) {
	original := RulesSlice{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
		AnyToolRule{Tool: "Read"},
		ExactRule{Tool: "Write", Input: "/tmp/file.txt"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var unmarshaled RulesSlice
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(unmarshaled) != len(original) {
		t.Fatalf("Length: got %d, want %d", len(unmarshaled), len(original))
	}

	// Check that each rule has the same String() representation
	for i := range original {
		if unmarshaled[i].String() != original[i].String() {
			t.Errorf("Rule %d: got %q, want %q", i, unmarshaled[i].String(), original[i].String())
		}

		// Check that each rule behaves the same
		testCases := []struct{ tool, input string }{
			{"Bash", "git"},
			{"Bash", "npm install"},
			{"Read", "/file"},
			{"Write", "/tmp/file.txt"},
		}
		for _, tc := range testCases {
			if original[i].Matches(tc.tool, tc.input) != unmarshaled[i].Matches(tc.tool, tc.input) {
				t.Errorf("Rule %d Matches(%q, %q) differs after round-trip", i, tc.tool, tc.input)
			}
		}
	}
}

// TestRulesSlice_MarshalNil tests marshaling a nil RulesSlice.
func TestRulesSlice_MarshalNil(t *testing.T) {
	var rules RulesSlice

	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	if string(data) != "null" {
		t.Errorf("Marshal(nil) = %s, want null", data)
	}
}

// TestRulesSlice_UnmarshalNull tests unmarshaling JSON null.
func TestRulesSlice_UnmarshalNull(t *testing.T) {
	var rules RulesSlice

	if err := json.Unmarshal([]byte("null"), &rules); err != nil {
		t.Fatalf("UnmarshalJSON(null) error = %v", err)
	}

	if rules != nil {
		t.Errorf("Unmarshal(null) = %+v, want nil", rules)
	}
}

// TestRulesSlice_UnmarshalEmptyArray tests unmarshaling an empty JSON array.
func TestRulesSlice_UnmarshalEmptyArray(t *testing.T) {
	var rules RulesSlice

	if err := json.Unmarshal([]byte("[]"), &rules); err != nil {
		t.Fatalf("UnmarshalJSON([]) error = %v", err)
	}

	if rules == nil || len(rules) != 0 {
		t.Errorf("Unmarshal([]) = %+v, want empty slice", rules)
	}
}

// TestRulesSlice_UnmarshalInvalidString tests unmarshaling an invalid rule string.
func TestRulesSlice_UnmarshalInvalidString(t *testing.T) {
	input := `["Bash(pwd)", "Bash(git *)", "invalid(rule"]`

	var rules RulesSlice
	err := json.Unmarshal([]byte(input), &rules)

	if err == nil {
		t.Error("Expected error for invalid rule string, got nil")
	}
}

// TestRulesSlice_Strings tests the Strings() method.
func TestRulesSlice_Strings(t *testing.T) {
	rules := RulesSlice{
		ExactRule{Tool: "Bash", Input: "git"},
		PrefixRule{Tool: "Bash", Prefix: "npm"},
	}

	strings := rules.Strings()

	expected := []string{"Bash(git)", "Bash(npm *)"}
	if len(strings) != len(expected) {
		t.Fatalf("Strings() length: got %d, want %d", len(strings), len(expected))
	}

	for i := range expected {
		if strings[i] != expected[i] {
			t.Errorf("Strings()[%d] = %q, want %q", i, strings[i], expected[i])
		}
	}
}

// TestRulesSlice_StringsNil tests Strings() on nil RulesSlice.
func TestRulesSlice_StringsNil(t *testing.T) {
	var rules RulesSlice

	strings := rules.Strings()

	if strings != nil {
		t.Errorf("Strings() on nil = %v, want nil", strings)
	}
}

// TestRulesSlice_MarshalInStruct tests marshaling RulesSlice as a struct field.
func TestRulesSlice_MarshalInStruct(t *testing.T) {
	type Container struct {
		Name  string     `json:"name"`
		Rules RulesSlice `json:"rules"`
	}

	container := Container{
		Name: "test",
		Rules: RulesSlice{
			ExactRule{Tool: "Bash", Input: "pwd"},
			PrefixRule{Tool: "Bash", Prefix: "ls"},
		},
	}

	data, err := json.Marshal(container)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	expected := `{"name":"test","rules":["Bash(pwd)","Bash(ls *)"]}`
	if string(data) != expected {
		t.Errorf("Marshal() = %s, want %s", data, expected)
	}
}

// TestRulesSlice_UnmarshalInStruct tests unmarshaling RulesSlice as a struct field.
func TestRulesSlice_UnmarshalInStruct(t *testing.T) {
	type Container struct {
		Name  string     `json:"name"`
		Rules RulesSlice `json:"rules"`
	}

	input := `{"name":"test","rules":["Bash(pwd)","Bash(ls *)"]}`

	var container Container
	if err := json.Unmarshal([]byte(input), &container); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if container.Name != "test" {
		t.Errorf("Name = %q, want test", container.Name)
	}

	if len(container.Rules) != 2 {
		t.Fatalf("Rules length = %d, want 2", len(container.Rules))
	}

	if container.Rules[0].String() != "Bash(pwd)" {
		t.Errorf("Rules[0] = %q, want Bash(pwd)", container.Rules[0].String())
	}

	if container.Rules[1].String() != "Bash(ls *)" {
		t.Errorf("Rules[1] = %q, want BASH(ls *)", container.Rules[1].String())
	}
}
