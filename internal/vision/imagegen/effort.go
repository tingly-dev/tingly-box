package imagegen

import (
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

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
	extra := make(map[string]any, len(params.ExtraFields())+1)
	for key, value := range params.ExtraFields() {
		extra[key] = value
	}
	extra[ReasoningEffortField] = effort
	params.SetExtraFields(extra)
}

func ReasoningEffort(params interface{ ExtraFields() map[string]any }) string {
	if effort, ok := params.ExtraFields()[ReasoningEffortField].(string); ok && effort != "" {
		return effort
	}
	return "auto"
}

// OpenAIImageExtras consumes the extension before an Images SDK call. Public
// OpenAI uses complexity for image planning; other compatible backends ignore
// the extension. Auto leaves the upstream default intact. Clone the map so
// adapting a call cannot mutate a retry.
func OpenAIImageExtras(provider *typ.Provider, model string, extras map[string]any) map[string]any {
	out := make(map[string]any, len(extras))
	for key, value := range extras {
		if key != ReasoningEffortField {
			out[key] = value
		}
	}
	if provider == nil || apiHost(provider.APIBase) != "api.openai.com" || !strings.HasPrefix(model, "gpt-image-") {
		return out
	}
	switch effort, _ := extras[ReasoningEffortField].(string); effort {
	case "low", "medium", "high":
		if _, exists := out["complexity"]; !exists {
			out["complexity"] = effort
		}
	}
	return out
}
