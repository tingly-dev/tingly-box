package decision

// RegisterDefaults registers the user-facing decision demo models:
//
//	decision-first   always the first option (confidence 1)
//	decision-stable  input-hashed choice, reproducible (confidence 0.8)
func RegisterDefaults(r *Registry) {
	_ = r.Register(NewMockModel(&MockModelConfig{
		ID:          "decision-first",
		Name:        "Decision (first option)",
		Description: "Answers every decisions request with the first option; predictable for routing demos and dry-runs.",
		Choose:      First,
	}))
	_ = r.Register(NewMockModel(&MockModelConfig{
		ID:          "decision-stable",
		Name:        "Decision (stable pick)",
		Description: "Picks an option by hashing the request: the same input always gets the same answer.",
		Choose:      Stable,
		Confidence:  0.8,
	}))
}
