package tui

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Shared pickers used across modes. Provider / Model / Rule pickers live
// alongside their primary mode (rule_mode.go, provider_mode.go) because
// they share helpers with the CRUD flows there. Scenario picker lives
// here because nothing else in those files needs it.

// pickScenario shows a Select over typ.BuiltinScenarios() with the given
// initial selection and returns the chosen scenario. ok is false when the
// user cancels or backs out (the caller should treat that as "abort the
// current flow"). The error is non-nil only for unexpected failures.
func pickScenario(initial typ.RuleScenario) (scn typ.RuleScenario, ok bool, err error) {
	scenarios := typ.BuiltinScenarios()
	items := make([]SelectItem[typ.RuleScenario], 0, len(scenarios))
	for _, s := range scenarios {
		items = append(items, SelectItem[typ.RuleScenario]{Title: string(s), Value: s})
	}
	if initial == "" {
		initial = typ.ScenarioOpenAI
	}
	r, err := Select("Scenario:", items, SelectOptions{
		CanGoBack: true,
		PageSize:  12,
		Initial:   initial,
	})
	if err != nil {
		return "", false, err
	}
	if r.IsCancel() || r.IsBack() {
		return "", false, nil
	}
	return r.Value, true, nil
}
