package anthropic

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// RegisterDefaults registers the default Anthropic-protocol virtual models
// into r: the Anthropic-only "virtual-claude-3" mock, the shared mocks
// ("echo-model", "ask-user-question", "ask-confirmation", "web-search-example"),
// and the compact transform models ("compact-thinking", "compact-round-only",
// "compact-round-files", "claude-code-compact", "claude-code-strategy").
func RegisterDefaults(r *Registry) {
	_ = r.Register(NewMockModel(&MockModelConfig{
		ID:      "virtual-claude-3",
		Name:    "Virtual Claude 3",
		Content: "Greetings! I'm a virtual Claude 3 model, providing fixed responses for testing and development purposes.",
		Delay:   150 * time.Millisecond,
	}))

	for _, spec := range vmodel.SharedDefaultMocks() {
		_ = r.Register(NewMockModel(&MockModelConfig{
			ID:       spec.ID,
			Name:     spec.Name,
			Content:  spec.Content,
			ToolCall: spec.ToolCall,
			Delay:    spec.Delay,
		}))
	}

	compactModels := []TransformModelConfig{
		{
			ID:          "compact-thinking",
			Name:        "Compact Thinking",
			Description: "Removes thinking blocks from historical conversation rounds (10-20% compression)",
			Chain:       transform.NewTransformChain([]transform.Transform{smart_compact.NewCompactTransform(2)}),
		},
		{
			ID:          "compact-round-only",
			Name:        "Compact Round Only",
			Description: "Keeps only user request + assistant conclusion, removes intermediate process (70-85% compression)",
			Chain:       transform.NewTransformChain([]transform.Transform{smart_compact.NewRoundOnlyTransform()}),
		},
		{
			ID:          "compact-round-files",
			Name:        "Compact Round Files",
			Description: "Keeps user/assistant + virtual file tools (75-88% compression)",
			Chain:       transform.NewTransformChain([]transform.Transform{smart_compact.NewRoundFilesTransform()}),
		},
		{
			ID:          "claude-code-compact",
			Name:        "Claude Code Compact",
			Description: "Conditional compression: only activates when last user message contains '<command>compact</command>' with tools defined.",
			Chain:       transform.NewTransformChain([]transform.Transform{NewClaudeCodeCompactTransform()}),
		},
		{
			ID:          "claude-code-strategy",
			Name:        "Claude Code Strategy",
			Description: "Applies DCP-inspired pruning strategies on every request: deduplicates repeated tool calls (keeps latest), and purges inputs of errored tool calls older than 4 turns.",
			Chain: transform.NewTransformChain([]transform.Transform{
				smart_compact.NewDeduplicationTransform(),
				smart_compact.NewPurgeErrorsTransform(4),
			}),
		},
	}
	for i := range compactModels {
		_ = r.Register(NewTransformModel(&compactModels[i]))
	}
}
