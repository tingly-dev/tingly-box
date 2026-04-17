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
			Name:        vm.GetID(), // Use ID as name since GetName() is not available
			Description: "Virtual model", // Generic description since GetDescription() is not available
			Type:        "virtual", // Generic type since GetType() is not available
			Metadata: map[string]interface{}{
				"vmType": "virtual",
			},
			Config: make(map[string]interface{}),
		}

		// Use a generic icon for all virtual models since type-specific methods are not available
		item.Icon = "memory"

		if err := extRegistry.RegisterItem(item); err != nil {
			// Log but continue with other items
			continue
		}
	}

	return nil
}
