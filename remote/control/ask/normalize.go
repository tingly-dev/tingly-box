package ask

// NormalizeQuestions coerces the heterogeneous "questions" payload into a
// canonical []map[string]any. Callers may serialize it as []interface{},
// []map[string]any, or []any of map[string]any — all should yield identical
// downstream rendering. Returns nil for any other shape (including nil).
func NormalizeQuestions(v any) []map[string]any {
	switch xs := v.(type) {
	case []map[string]any:
		return xs
	case []interface{}:
		out := make([]map[string]any, 0, len(xs))
		for _, item := range xs {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			} else if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// NormalizeOptions is identical to NormalizeQuestions; named separately for
// readability at call sites where the payload represents per-question options.
func NormalizeOptions(v any) []map[string]any { return NormalizeQuestions(v) }
