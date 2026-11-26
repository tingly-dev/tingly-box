package spec

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// DefaultDefinitions converts all shipped user-facing models to the common
// definition. Plain responses are exported as data; transforms keep explicit
// behavior references so their request-dependent Go logic stays intact.
func DefaultDefinitions() []Definition {
	svc := virtualserver.NewService()
	byID := map[string]Definition{}
	for _, s := range vmodel.SharedDefaultMocks() {
		byID[s.ID] = FromMock(s)
	}
	for _, s := range vmodel.DefaultSequenceConfigs() {
		byID[s.ID] = FromSequence(s)
	}
	for _, p := range []struct {
		format scenario.ResponseFormat
		models []vmodel.VirtualModel
	}{
		{scenario.FormatOpenAIChat, baseModels(svc.GetOpenAIRegistry().List())},
		{scenario.FormatAnthropic, baseModels(svc.GetAnthropicRegistry().List())},
	} {
		for _, m := range p.models {
			d, ok := byID[m.GetID()]
			if !ok {
				if exporter, ok := m.(interface{ Spec() vmodel.SharedMockSpec }); ok {
					d = FromMock(exporter.Spec())
				} else if m.GetType() == vmodel.VirtualModelTypeProxy {
					d = Definition{ID: m.GetID(), Name: m.GetName(), Description: m.GetDescription(), Kind: Transform, Transform: &TransformSpec{Builtin: m.GetID()}}
				} else {
					d = Definition{ID: m.GetID(), Name: m.GetName(), Description: m.GetDescription(), Kind: Builtin, Builtin: "model/" + m.GetID()}
				}
			}
			d.Protocols = append(d.Protocols, p.format)
			if p.format == scenario.FormatOpenAIChat {
				d.Protocols = append(d.Protocols, scenario.FormatOpenAIResponses)
			}
			byID[d.ID] = d
		}
	}
	for _, m := range svc.GetDecisionRegistry().List() {
		if m.GetID() != "decision-first" && m.GetID() != "decision-stable" {
			byID[m.GetID()] = Definition{ID: m.GetID(), Name: m.GetName(), Description: m.GetDescription(), Kind: Builtin, Builtin: "model/" + m.GetID(), Protocols: []scenario.ResponseFormat{FormatDecisions}}
			continue
		}
		strategy, confidence := "first", float64(0)
		if m.GetID() == "decision-stable" {
			strategy = "stable"
			confidence = .8
		}
		byID[m.GetID()] = Definition{ID: m.GetID(), Name: m.GetName(), Description: m.GetDescription(), Kind: Decision, Decision: &DecisionSpec{Strategy: strategy, Confidence: confidence}, Protocols: []scenario.ResponseFormat{FormatDecisions}}
	}
	return sortedDefinitions(byID)
}
func baseModels[T vmodel.VirtualModel](models []T) []vmodel.VirtualModel {
	out := make([]vmodel.VirtualModel, len(models))
	for i, m := range models {
		out[i] = m
	}
	return out
}
func sortedDefinitions(values map[string]Definition) []Definition {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Definition, 0, len(ids))
	for _, id := range ids {
		out = append(out, values[id])
	}
	return out
}

// AllDefinitions is the migration/export inventory, including opt-in error and
// usage fixtures and the request-dependent built-in matrix scenarios. It is
// deliberately not installed into production defaults.
func AllDefinitions() []Definition {
	values := map[string]Definition{}
	for _, d := range DefaultDefinitions() {
		values[d.ID] = d
	}
	for _, list := range [][]vmodel.SharedMockSpec{vmodel.StreamTestMockSpecs(), vmodel.ExtendedErrorSpecs()} {
		for _, s := range list {
			if _, ok := values[s.ID]; !ok {
				values[s.ID] = FromMock(s)
			}
		}
	}
	for _, s := range scenario.AllScenarios() {
		id := s.Name
		if _, exists := values[id]; exists {
			id = "scenario-" + s.Name
		}
		expect, structural := scenario.BuiltinExpectations(s.Name)
		values[id] = Definition{ID: id, Name: s.Name, Description: s.Description, Kind: Builtin, Builtin: "scenario/" + s.Name, Tags: s.Tags, SkipTransitive: s.SkipTransitive, Expect: expect, Structural: structural}
	}
	return sortedDefinitions(values)
}

// Export emits the same validated contract accepted by Load. No function is
// silently serialized or discarded: custom Go callbacks need a catalog name.
func Export(defs []Definition, format string) ([]byte, error) {
	// Validate with the public decoder so Go and file definitions agree.
	normalized := make([]Definition, 0, len(defs))
	seen := map[string]bool{}
	for _, original := range defs {
		raw, err := json.Marshal(original)
		if err != nil {
			return nil, err
		}
		d, err := decodeOne(raw)
		if err != nil {
			return nil, err
		}
		if seen[d.ID] {
			return nil, fmt.Errorf("duplicate definition id %q", d.ID)
		}
		seen[d.ID] = true
		normalized = append(normalized, d)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("definitions must not be empty")
	}

	switch format {
	case "json":
		return json.MarshalIndent(normalized, "", "  ")
	case "yaml", "yml":
		return marshalYAML(normalized)
	default:
		return nil, fmt.Errorf("export format must be json, yaml or yml")
	}
}
