package guardrails

// Scope limits when a rule is applied.
type Scope struct {
	Scenarios  []string      `json:"scenarios,omitempty" yaml:"scenarios,omitempty"`
	Models     []string      `json:"models,omitempty" yaml:"models,omitempty"`
	Directions []Direction   `json:"directions,omitempty" yaml:"directions,omitempty"`
	Tags       []string      `json:"tags,omitempty" yaml:"tags,omitempty"`
	Content    []ContentType `json:"content_types,omitempty" yaml:"content_types,omitempty"`
}

// Matches returns true when the input matches scope constraints.
func (s Scope) Matches(input Input) bool {
	if len(s.Scenarios) > 0 && !stringInSlice(input.Scenario, s.Scenarios) {
		return false
	}
	if len(s.Models) > 0 && !stringInSlice(input.Model, s.Models) {
		return false
	}
	if len(s.Directions) > 0 && !directionInSlice(input.Direction, s.Directions) {
		return false
	}
	if len(s.Tags) > 0 && !anyTagMatches(input.Tags, s.Tags) {
		return false
	}
	if len(s.Content) > 0 && !input.Content.HasAny(s.Content) {
		return false
	}
	return true
}

func stringInSlice(value string, items []string) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func directionInSlice(value Direction, items []Direction) bool {
	for _, item := range items {
		if value == item {
			return true
		}
	}
	return false
}

func anyTagMatches(inputTags, scopeTags []string) bool {
	for _, tag := range inputTags {
		if stringInSlice(tag, scopeTags) {
			return true
		}
	}
	return false
}
