// Package feature provides experimental feature flag definitions and parsing.
package feature

import "strings"

const (
	// FeatureCompact enables smart compact for conversation history.
	FeatureCompact = "compact"
)

// KnownFeatures is the set of all recognized experimental features.
var KnownFeatures = map[string]bool{
	FeatureCompact: true,
}

// ParseFeatures parses a comma-separated string of feature names into a map.
// Empty values are ignored. Unknown features are silently ignored.
// Example: "compact,other" -> {"compact": true, "other": true}
func ParseFeatures(expr string) map[string]bool {
	features := make(map[string]bool)
	if expr == "" {
		return features
	}

	for _, part := range split(expr) {
		if part == "" {
			continue
		}
		features[part] = true
	}
	return features
}

// IsEnabled checks if a specific feature is enabled in the features map.
func IsEnabled(features map[string]bool, feature string) bool {
	return features[feature]
}

// split is a simple comma split that handles whitespace.
func split(s string) []string {
	var parts []string
	var current []rune

	for _, r := range s {
		switch r {
		case ',':
			part := strings.TrimSpace(string(current))
			if part != "" {
				parts = append(parts, part)
			}
			current = nil
		default:
			current = append(current, r)
		}
	}
	if len(current) > 0 {
		part := strings.TrimSpace(string(current))
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
