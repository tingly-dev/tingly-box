package feature

import (
	"testing"
)

func TestParseFeatures(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected map[string]bool
	}{
		{
			name:     "empty string",
			expr:     "",
			expected: map[string]bool{},
		},
		{
			name:     "single feature",
			expr:     "compact",
			expected: map[string]bool{"compact": true},
		},
		{
			name:     "multiple features",
			expr:     "compact,other",
			expected: map[string]bool{"compact": true, "other": true},
		},
		{
			name:     "multiple features with whitespace",
			expr:     "compact, other , test",
			expected: map[string]bool{"compact": true, "other": true, "test": true},
		},
		{
			name:     "features with empty values",
			expr:     "compact,,other",
			expected: map[string]bool{"compact": true, "other": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseFeatures(tt.expr)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseFeatures() = %v, want %v", result, tt.expected)
			}
			for k := range tt.expected {
				if !result[k] {
					t.Errorf("ParseFeatures() missing key %s", k)
				}
			}
		})
	}
}

func TestIsEnabled(t *testing.T) {
	features := map[string]bool{
		FeatureCompact: true,
		"other":        false,
	}

	tests := []struct {
		name     string
		feature  string
		expected bool
	}{
		{
			name:     "enabled feature",
			feature:  FeatureCompact,
			expected: true,
		},
		{
			name:     "disabled feature",
			feature:  "other",
			expected: false,
		},
		{
			name:     "unknown feature",
			feature:  "unknown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsEnabled(features, tt.feature)
			if result != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestKnownFeatures(t *testing.T) {
	if !KnownFeatures[FeatureCompact] {
		t.Errorf("FeatureCompact not in KnownFeatures")
	}
}
