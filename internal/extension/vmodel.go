package extension

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/virtualmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/virtualmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/virtualmodel/openai"
)

// RegisterVModelExtension registers VModel as an extension in the extension
// registry, walking both per-provider virtual model registries. Each item is
// tagged with a "provider" metadata key ("anthropic" or "openai") so the UI
// can render the split. When the same model ID exists in both registries,
// only the first one (anthropic) is registered as an item — the second is
// skipped to avoid duplicate item IDs.
func RegisterVModelExtension(extRegistry *ExtensionRegistry, anthropicReg *anthropicvm.Registry, openaiReg *openaivm.Registry) error {
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

	for _, vm := range anthropicReg.List() {
		registerVModelItem(extRegistry, vm.GetID(), vm.GetName(), vm.GetDescription(), vm.GetType(), "anthropic")
	}
	for _, vm := range openaiReg.List() {
		if extRegistry.GetItem("vmodel", vm.GetID()) != nil {
			continue
		}
		registerVModelItem(extRegistry, vm.GetID(), vm.GetName(), vm.GetDescription(), vm.GetType(), "openai")
	}

	return nil
}

func registerVModelItem(extRegistry *ExtensionRegistry, id, name, description string, vmType virtualmodel.VirtualModelType, provider string) {
	item := &ExtensionItem{
		ID:          id,
		ExtensionID: "vmodel",
		Name:        name,
		Description: description,
		Type:        string(vmType),
		Metadata: map[string]interface{}{
			"vmType":   string(vmType),
			"provider": provider,
		},
		Config: make(map[string]interface{}),
	}

	switch vmType {
	case virtualmodel.VirtualModelTypeStatic:
		item.Icon = "message"
	case virtualmodel.VirtualModelTypeProxy:
		item.Icon = "compress"
	case virtualmodel.VirtualModelTypeTool:
		item.Icon = "build"
	}

	_ = extRegistry.RegisterItem(item)
}
