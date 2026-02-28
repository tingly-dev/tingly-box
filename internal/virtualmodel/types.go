package virtualmodel

import (
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Model represents a virtual model in the models list (OpenAI-compatible format)
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// VirtualModelConfig holds the configuration for a virtual model
type VirtualModelConfig struct {
	ID           string
	Name         string
	Description  string
	Content      string
	Role         string
	FinishReason string
	Delay        time.Duration
	StreamChunks []string // For streaming: chunks to send

	// Proxy mode fields
	IsProxy      bool                // Is this a proxy virtual model?
	DelegateModel string              // Real model to delegate to (e.g., "claude-3-5-sonnet-20241022")
	Transformer  protocol.Transformer // Optional transformer for proxy mode
}

// VirtualModel represents a registered virtual model
type VirtualModel struct {
	config *VirtualModelConfig
}

// NewVirtualModel creates a new virtual model
func NewVirtualModel(cfg *VirtualModelConfig) *VirtualModel {
	if cfg.Role == "" {
		cfg.Role = "assistant"
	}
	if cfg.FinishReason == "" {
		cfg.FinishReason = "stop"
	}
	return &VirtualModel{config: cfg}
}

// GetID returns the model ID
func (vm *VirtualModel) GetID() string {
	return vm.config.ID
}

// GetName returns the model name
func (vm *VirtualModel) GetName() string {
	if vm.config.Name != "" {
		return vm.config.Name
	}
	return vm.config.ID
}

// GetContent returns the response content
func (vm *VirtualModel) GetContent() string {
	return vm.config.Content
}

// GetDelay returns the response delay
func (vm *VirtualModel) GetDelay() time.Duration {
	return vm.config.Delay
}

// GetStreamChunks returns the streaming chunks
func (vm *VirtualModel) GetStreamChunks() []string {
	if len(vm.config.StreamChunks) == 0 {
		// Default: split content into words for streaming
		return splitIntoChunks(vm.config.Content)
	}
	return vm.config.StreamChunks
}

// IsProxy returns whether this is a proxy virtual model
func (vm *VirtualModel) IsProxy() bool {
	return vm.config.IsProxy
}

// GetDelegateModel returns the delegate model for proxy mode
func (vm *VirtualModel) GetDelegateModel() string {
	return vm.config.DelegateModel
}

// GetTransformer returns the transformer for proxy mode
func (vm *VirtualModel) GetTransformer() protocol.Transformer {
	return vm.config.Transformer
}

// ToModel converts to Model type for API response
func (vm *VirtualModel) ToModel() Model {
	return Model{
		ID:      vm.config.ID,
		Object:  "model",
		Created: time.Now().Unix(),
		OwnedBy: "tingly-box-virtual",
	}
}

// splitIntoChunks splits content into word-based chunks for streaming
func splitIntoChunks(content string) []string {
	words := []string{}
	currentWord := ""
	for _, ch := range content {
		if ch == ' ' || ch == '\n' || ch == '\t' {
			if currentWord != "" {
				words = append(words, currentWord)
				currentWord = ""
			}
			words = append(words, string(ch))
		} else {
			currentWord += string(ch)
		}
	}
	if currentWord != "" {
		words = append(words, currentWord)
	}
	// Add some grouping to make chunks more realistic
	chunks := []string{}
	currentChunk := ""
	for i, word := range words {
		currentChunk += word
		if (i+1)%3 == 0 || i == len(words)-1 {
			chunks = append(chunks, currentChunk)
			currentChunk = ""
		}
	}
	if len(chunks) == 0 {
		chunks = append(chunks, content)
	}
	return chunks
}
