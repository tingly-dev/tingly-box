package virtualmodel

import (
	"fmt"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// Registry manages virtual models indexed by ID and by protocol type.
// At Register time it reads vm.Protocols() to build a protocol index and
// validates that each declared APIType maps to the corresponding sub-interface.
type Registry struct {
	models     map[string]VirtualModel
	byProtocol map[protocol.APIType]map[string]VirtualModel
	mu         sync.RWMutex
}

// NewRegistry creates a new virtual model registry.
func NewRegistry() *Registry {
	return &Registry{
		models:     make(map[string]VirtualModel),
		byProtocol: make(map[protocol.APIType]map[string]VirtualModel),
	}
}

// Register registers a virtual model.
// It reads vm.Protocols() to populate the protocol index and validates that
// each declared APIType corresponds to an implemented sub-interface:
//   - TypeAnthropicV1 / TypeAnthropicBeta → AnthropicVirtualModel
//   - TypeOpenAIChat / TypeOpenAIResponses → OpenAIChatVirtualModel
func (r *Registry) Register(vm VirtualModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := vm.GetID()
	if _, exists := r.models[id]; exists {
		return fmt.Errorf("model already registered: %s", id)
	}

	for _, apiType := range vm.Protocols() {
		if err := validateProtocol(vm, apiType); err != nil {
			return fmt.Errorf("model %s: %w", id, err)
		}
		// TypeAnthropicV1 is compatible with Beta; route it into the Beta bucket
		// so GetAnthropicVM can find it without a separate v1 getter.
		indexType := apiType
		if apiType == protocol.TypeAnthropicV1 {
			indexType = protocol.TypeAnthropicBeta
		}
		if r.byProtocol[indexType] == nil {
			r.byProtocol[indexType] = make(map[string]VirtualModel)
		}
		r.byProtocol[indexType][id] = vm
	}

	r.models[id] = vm
	return nil
}

// validateProtocol checks that vm implements the sub-interface required by apiType.
func validateProtocol(vm VirtualModel, apiType protocol.APIType) error {
	switch apiType {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		if _, ok := vm.(AnthropicVirtualModel); !ok {
			return fmt.Errorf("declares %s but does not implement AnthropicVirtualModel", apiType)
		}
	case protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses:
		if _, ok := vm.(OpenAIChatVirtualModel); !ok {
			return fmt.Errorf("declares %s but does not implement OpenAIChatVirtualModel", apiType)
		}
	}
	return nil
}

// Unregister removes a virtual model from all indexes.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	vm, exists := r.models[id]
	if !exists {
		return
	}
	for _, apiType := range vm.Protocols() {
		indexType := apiType
		if apiType == protocol.TypeAnthropicV1 {
			indexType = protocol.TypeAnthropicBeta
		}
		delete(r.byProtocol[indexType], id)
	}
	delete(r.models, id)
}

// Get retrieves a virtual model by ID (protocol-agnostic).
func (r *Registry) Get(id string) VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[id]
}

// GetAnthropicVM returns the AnthropicVirtualModel for id, or nil if not found
// or if the model does not support the Anthropic protocol.
func (r *Registry) GetAnthropicVM(id string) AnthropicVirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vm := r.byProtocol[protocol.TypeAnthropicBeta][id]
	if vm == nil {
		return nil
	}
	avm, _ := vm.(AnthropicVirtualModel)
	return avm
}

// GetOpenAIChatVM returns the OpenAIChatVirtualModel for id, or nil if not found
// or if the model does not support the OpenAI Chat protocol.
func (r *Registry) GetOpenAIChatVM(id string) OpenAIChatVirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vm := r.byProtocol[protocol.TypeOpenAIChat][id]
	if vm == nil {
		return nil
	}
	ovm, _ := vm.(OpenAIChatVirtualModel)
	return ovm
}

// ListModels returns all registered models as Model slices.
func (r *Registry) ListModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]Model, 0, len(r.models))
	for _, vm := range r.models {
		models = append(models, vm.ToModel())
	}
	return models
}

// List returns all registered virtual models.
func (r *Registry) List() []VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vms := make([]VirtualModel, 0, len(r.models))
	for _, vm := range r.models {
		vms = append(vms, vm)
	}
	return vms
}

// Clear clears all registered models from all indexes.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models = make(map[string]VirtualModel)
	r.byProtocol = make(map[protocol.APIType]map[string]VirtualModel)
}

// RegisterDefaults registers default virtual models.
func (r *Registry) RegisterDefaults() {
	staticModels := []MockModelConfig{
		{
			ID:      "virtual-gpt-4",
			Name:    "Virtual GPT-4",
			Content: "Hello! This is a response from the virtual GPT-4 model. I'm here to help you test your application without making actual API calls.",
			Delay:   100 * time.Millisecond,
		},
		{
			ID:      "virtual-claude-3",
			Name:    "Virtual Claude 3",
			Content: "Greetings! I'm a virtual Claude 3 model, providing fixed responses for testing and development purposes.",
			Delay:   150 * time.Millisecond,
		},
		{
			ID:      "echo-model",
			Name:    "Echo Model",
			Content: "Echo: Your message has been received by the virtual model.",
			Delay:   50 * time.Millisecond,
		},
	}
	for i := range staticModels {
		_ = r.Register(NewMockModel(&staticModels[i]))
	}

	r.registerToolModels()
	r.registerCompactModels()
}

func (r *Registry) registerToolModels() {
	toolModels := []MockModelConfig{
		{
			ID:   "ask-user-question",
			Name: "Ask User Question",
			ToolCall: &ToolCallConfig{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Which approach would you prefer?",
					"options": []map[string]string{
						{"label": "Fast Mode", "value": "fast", "description": "Quick results with less accuracy"},
						{"label": "Accurate Mode", "value": "accurate", "description": "Slower but more accurate results"},
					},
				},
			},
			Delay: 100 * time.Millisecond,
		},
		{
			ID:   "ask-confirmation",
			Name: "Ask Confirmation",
			ToolCall: &ToolCallConfig{
				Name: "ask_user_question",
				Arguments: map[string]interface{}{
					"question": "Please confirm to proceed:",
					"options": []map[string]string{
						{"label": "Yes", "value": "yes", "description": "Proceed with the action"},
						{"label": "No", "value": "no", "description": "Cancel the action"},
					},
				},
			},
			Delay: 50 * time.Millisecond,
		},
		{
			ID:   "web-search-example",
			Name: "Web Search Example",
			ToolCall: &ToolCallConfig{
				Name:      "web_search",
				Arguments: map[string]interface{}{"query": "latest AI developments"},
			},
			Delay: 50 * time.Millisecond,
		},
	}
	for i := range toolModels {
		_ = r.Register(NewMockModel(&toolModels[i]))
	}
}

func (r *Registry) registerCompactModels() {
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
