package extension

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// RegisterVModelExtension registers VModel as an extension in the extension registry
func RegisterVModelExtension(extRegistry *ExtensionRegistry, vmRegistry *virtualmodel.Registry) error {
	// Register VModel extension
	vmodelExt := &Extension{
		ID:          "vmodel",
		Name:        "Virtual Models",
		Description: "Virtual models for testing, compression, and tool simulation",
		Icon:        "memory",
		Metadata: map[string]string{
			"version": "1.0",
		},
	}

	if err := extRegistry.RegisterExtension(vmodelExt); err != nil {
		return fmt.Errorf("failed to register vmodel extension: %w", err)
	}

	// Register all VirtualModels as ExtensionItems
	virtualModels := vmRegistry.List()
	for _, vm := range virtualModels {
		item := &ExtensionItem{
			ID:          vm.GetID(),
			ExtensionID: "vmodel",
			Name:        vm.GetName(),
			Description: vm.GetDescription(),
			Type:        string(vm.GetType()),
			Metadata: map[string]interface{}{
				"vmType": string(vm.GetType()),
			},
			Config: make(map[string]interface{}),
		}

		// Add type-specific metadata
		switch vm.GetType() {
		case virtualmodel.VirtualModelTypeStatic:
			item.Icon = "message"
		case virtualmodel.VirtualModelTypeProxy:
			item.Icon = "compress"
			// Add delegate model info if set
			if delegate := vm.GetDelegateModel(); delegate != "" {
				item.Metadata["delegateModel"] = delegate
			}
		case virtualmodel.VirtualModelTypeTool:
			item.Icon = "build"
			// Add tool call info if set
			if toolCall := vm.GetToolCall(); toolCall != nil {
				item.Metadata["toolName"] = toolCall.Name
				item.Metadata["toolArguments"] = toolCall.Arguments
			}
		}

		if err := extRegistry.RegisterItem(item); err != nil {
			// Log but continue with other items
			continue
		}
	}

	return nil
}
