package virtualmodel

import (
	"fmt"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// Registry manages virtual models
type Registry struct {
	models map[string]*VirtualModel
	mu     sync.RWMutex
}

// NewRegistry creates a new virtual model registry
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[string]*VirtualModel),
	}
}

// Register registers a virtual model
func (r *Registry) Register(vm *VirtualModel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := vm.GetID()
	if _, exists := r.models[id]; exists {
		return fmt.Errorf("model already registered: %s", id)
	}

	r.models[id] = vm
	return nil
}

// Unregister unregisters a virtual model
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.models, id)
}

// Get retrieves a virtual model by ID
func (r *Registry) Get(id string) *VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.models[id]
}

// ListModels returns all registered models as Model slices
func (r *Registry) ListModels() []Model {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]Model, 0, len(r.models))
	for _, vm := range r.models {
		models = append(models, vm.ToModel())
	}
	return models
}

// List returns all registered virtual models
func (r *Registry) List() []*VirtualModel {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vms := make([]*VirtualModel, 0, len(r.models))
	for _, vm := range r.models {
		vms = append(vms, vm)
	}
	return vms
}

// Clear clears all registered models
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.models = make(map[string]*VirtualModel)
}

// RegisterDefaults registers default virtual models
func (r *Registry) RegisterDefaults() {
	defaultModels := []*VirtualModelConfig{
		// Mock models for testing
		{
			ID:          "virtual-gpt-4",
			Name:        "Virtual GPT-4",
			Description: "A virtual model that returns fixed responses for testing",
			Content:     "Hello! This is a response from the virtual GPT-4 model. I'm here to help you test your application without making actual API calls.",
			Delay:       100 * 1000000, // 100ms
		},
		{
			ID:          "virtual-claude-3",
			Name:        "Virtual Claude 3",
			Description: "A virtual model simulating Claude 3 responses",
			Content:     "Greetings! I'm a virtual Claude 3 model, providing fixed responses for testing and development purposes.",
			Delay:       150 * 1000000, // 150ms
		},
		{
			ID:          "echo-model",
			Name:        "Echo Model",
			Description: "A model that echoes back a simple message",
			Content:     "Echo: Your message has been received by the virtual model.",
			Delay:       50 * 1000000, // 50ms
		},
	}

	for _, cfg := range defaultModels {
		vm := NewVirtualModel(cfg)
		if err := r.Register(vm); err != nil {
			// Log but continue
			continue
		}
	}

	// Register compact proxy models
	r.registerCompactModels()

	// Register tool-type models
	r.registerToolModels()

	// Register vision analyzer model for server tool testing
	r.registerVisionModels()
}

// registerToolModels registers tool-type virtual models
func (r *Registry) registerToolModels() {
	toolModels := []*VirtualModelConfig{
		{
			ID:          "ask-user-question",
			Name:        "Ask User Question",
			Description: "A virtual model that asks the user a question with predefined options",
			Type:        VirtualModelTypeTool,
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
			Delay: 100 * 1000000, // 100ms
		},
		{
			ID:          "ask-confirmation",
			Name:        "Ask Confirmation",
			Description: "A virtual model that asks for user confirmation",
			Type:        VirtualModelTypeTool,
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
			Delay: 50 * 1000000, // 50ms
		},
		// Example of a different tool type
		{
			ID:          "web-search-example",
			Name:        "Web Search Example",
			Description: "A virtual model that demonstrates web_search tool call",
			Type:        VirtualModelTypeTool,
			ToolCall: &ToolCallConfig{
				Name: "web_search",
				Arguments: map[string]interface{}{
					"query": "latest AI developments",
				},
			},
			Delay: 50 * 1000000,
		},
	}

	for _, cfg := range toolModels {
		vm := NewVirtualModel(cfg)
		if err := r.Register(vm); err != nil {
			// Log but continue
			continue
		}
	}
}

// registerCompactModels registers compact compression virtual models
func (r *Registry) registerCompactModels() {
	compactModels := []*VirtualModelConfig{
		{
			ID:            "compact-thinking",
			Name:          "Compact Thinking",
			Description:   "Removes thinking blocks from historical conversation rounds (10-20% compression)",
			Type:          VirtualModelTypeProxy,
			DelegateModel: "", // User should specify the real model
			Transformer:   newSmartCompactTransformer(),
		},
		{
			ID:            "compact-round-only",
			Name:          "Compact Round Only",
			Description:   "Keeps only user request + assistant conclusion, removes intermediate process (70-85% compression)",
			Type:          VirtualModelTypeProxy,
			DelegateModel: "",
			Transformer:   smart_compact.NewRoundOnlyTransformer(),
		},
		{
			ID:            "compact-round-files",
			Name:          "Compact Round Files",
			Description:   "Keeps user/assistant + virtual file tools (75-88% compression)",
			Type:          VirtualModelTypeProxy,
			DelegateModel: "",
			Transformer:   smart_compact.NewRoundFilesTransformer(),
		},
	}

	for _, cfg := range compactModels {
		vm := NewVirtualModel(cfg)
		if err := r.Register(vm); err != nil {
			// Log but continue
			continue
		}
	}
}

// newSmartCompactTransformer creates a smart_compact transformer with default settings
func newSmartCompactTransformer() protocol.Transformer {
	// Create smart_compact transformer with keepLastNRounds=2
	return smart_compact.NewCompactTransformer(2)
}

// registerVisionModels registers virtual models for vision/image analysis testing
func (r *Registry) registerVisionModels() {
	visionModels := []*VirtualModelConfig{
		{
			ID:   "vision-analyzer",
			Name: "Vision Analyzer",
			Description: "A virtual model that simulates vision analysis. Returns a canned image " +
				"description for testing the server tool framework's analyze_image tool.",
			Content: "The image shows a Go source code file displayed in a code editor. " +
				"The file contains an HTTP handler function named `handleRequest` with approximately 30 lines of code. " +
				"Key observations:\n" +
				"- Line 15: There is an unhandled error from `db.Query()` — the `err` variable is assigned but never checked.\n" +
				"- Line 22: A SQL query string is constructed using string concatenation, which is vulnerable to SQL injection.\n" +
				"- Line 28: The function returns `http.StatusOK` even when the query fails.\n" +
				"- The function accepts `w http.ResponseWriter` and `r *http.Request` as parameters.\n" +
				"- Import block includes `database/sql`, `net/http`, and `encoding/json`.",
			Delay: 200 * 1000000, // 200ms to simulate vision model latency
		},
	}

	for _, cfg := range visionModels {
		vm := NewVirtualModel(cfg)
		if err := r.Register(vm); err != nil {
			continue
		}
	}
}
