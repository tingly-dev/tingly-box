package agentsec

import (
	"encoding/json"
	"fmt"
)

// RulesSlice is a helper type for JSON marshaling of []Rule.
// It serializes to a JSON array of rule strings and deserializes back to []Rule.
type RulesSlice []Rule

// MarshalJSON implements json.Marshaler for RulesSlice.
// It converts each Rule to its string representation.
func (rs RulesSlice) MarshalJSON() ([]byte, error) {
	if rs == nil {
		return []byte("null"), nil
	}

	strings := make([]string, len(rs))
	for i, rule := range rs {
		strings[i] = rule.String()
	}

	return json.Marshal(strings)
}

// UnmarshalJSON implements json.Unmarshaler for RulesSlice.
// It parses each string back into a Rule using ParseRuleToRule.
func (rs *RulesSlice) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*rs = nil
		return nil
	}

	var strings []string
	if err := json.Unmarshal(data, &strings); err != nil {
		return fmt.Errorf("failed to unmarshal rule strings: %w", err)
	}

	rules := make([]Rule, 0, len(strings))
	for _, s := range strings {
		rule, err := ParseRule(s)
		if err != nil {
			return fmt.Errorf("failed to parse rule %q: %w", s, err)
		}
		rules = append(rules, rule)
	}

	*rs = rules
	return nil
}

// Strings returns the rule slice as a slice of strings (for display/debugging).
func (rs RulesSlice) Strings() []string {
	if rs == nil {
		return nil
	}

	strings := make([]string, len(rs))
	for i, rule := range rs {
		strings[i] = rule.String()
	}
	return strings
}
