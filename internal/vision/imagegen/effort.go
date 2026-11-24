package imagegen

import "maps"

// ReasoningEffortField is a gateway extension shared by generation and edit
// requests. It carries the caller's choice; only the serving adapter decides
// whether to use it. SDK extras avoid changing the SDK fork.
const ReasoningEffortField = "reasoning_effort"

type effortParams interface {
	ExtraFields() map[string]any
	SetExtraFields(map[string]any)
}

func SetReasoningEffort(params effortParams, effort string) {
	if effort == "" {
		return
	}
	extra := maps.Clone(params.ExtraFields())
	if extra == nil {
		extra = make(map[string]any)
	}
	extra[ReasoningEffortField] = effort
	params.SetExtraFields(extra)
}

func ReasoningEffort(params effortParams) string {
	if effort, ok := params.ExtraFields()[ReasoningEffortField].(string); ok && effort != "" {
		return effort
	}
	return "auto"
}

// OmitReasoningEffort keeps the gateway hint off unsupported upstream wires.
// Copy extras because SDK params are passed by value but their maps are shared.
func OmitReasoningEffort(params effortParams) {
	if _, exists := params.ExtraFields()[ReasoningEffortField]; !exists {
		return
	}
	extra := maps.Clone(params.ExtraFields())
	delete(extra, ReasoningEffortField)
	params.SetExtraFields(extra)
}
