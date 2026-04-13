package extension

import (
	"fmt"
)

// ExtensionService combines registry and store for unified extension management
type ExtensionService struct {
	registry *ExtensionRegistry
	store    *ExtensionStore
}

// NewExtensionService creates a new extension service
func NewExtensionService(registry *ExtensionRegistry, store *ExtensionStore) *ExtensionService {
	return &ExtensionService{
		registry: registry,
		store:    store,
	}
}

// ListExtensions returns all extensions with merged state from registry and store
func (s *ExtensionService) ListExtensions() ([]*ExtensionView, error) {
	extensions := s.registry.ListExtensions()

	views := make([]*ExtensionView, 0, len(extensions))
	for _, ext := range extensions {
		view, err := s.buildExtensionView(ext)
		if err != nil {
			return nil, fmt.Errorf("failed to build view for %s: %w", ext.ID, err)
		}
		views = append(views, view)
	}

	return views, nil
}

// GetExtension returns a specific extension with merged state
func (s *ExtensionService) GetExtension(id string) (*ExtensionView, error) {
	ext := s.registry.GetExtension(id)
	if ext == nil {
		return nil, fmt.Errorf("extension not found: %s", id)
	}

	return s.buildExtensionView(ext)
}

// UpdateExtension updates extension configuration (enabled, order)
func (s *ExtensionService) UpdateExtension(id string, enabled *bool, order *int) error {
	// Check if extension exists
	ext := s.registry.GetExtension(id)
	if ext == nil {
		return fmt.Errorf("extension not found: %s", id)
	}

	// Get existing config or create new
	config, err := s.store.GetConfig(id)
	if err != nil {
		// Config doesn't exist, create new with defaults
		config = &ExtensionConfig{
			ID:      id,
			Type:    "extension",
			Enabled: true, // default
			Order:   0,    // default
		}
	}

	// Update fields if provided
	if enabled != nil {
		config.Enabled = *enabled
	}
	if order != nil {
		config.Order = *order
	}

	// Save to store
	if err := s.store.SetConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// GetItem returns a specific item with merged state
func (s *ExtensionService) GetItem(extensionID, itemID string) (*ExtensionItemView, error) {
	// Check if extension exists
	ext := s.registry.GetExtension(extensionID)
	if ext == nil {
		return nil, fmt.Errorf("extension not found: %s", extensionID)
	}

	// Check if item exists
	item := s.registry.GetItem(extensionID, itemID)
	if item == nil {
		return nil, fmt.Errorf("item not found: %s/%s", extensionID, itemID)
	}

	return s.buildItemView(item)
}

// UpdateItem updates item configuration (enabled, config)
func (s *ExtensionService) UpdateItem(extensionID, itemID string, enabled *bool, configJSON *string) error {
	// Check if extension exists
	if s.registry.GetExtension(extensionID) == nil {
		return fmt.Errorf("extension not found: %s", extensionID)
	}

	// Check if item exists
	if s.registry.GetItem(extensionID, itemID) == nil {
		return fmt.Errorf("item not found: %s/%s", extensionID, itemID)
	}

	// Get existing config or create new
	config, err := s.store.GetConfig(itemID)
	if err != nil {
		// Config doesn't exist, create new with defaults
		config = &ExtensionConfig{
			ID:       itemID,
			Type:     "item",
			ParentID: extensionID,
			Enabled:  true, // default
			Config:   "",   // default
			Order:    0,    // default
		}
	}

	// Update fields if provided
	if enabled != nil {
		config.Enabled = *enabled
	}
	if configJSON != nil {
		config.Config = *configJSON
	}

	// Save to store
	if err := s.store.SetConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// ListItemsByExtension returns all items for an extension with merged state
func (s *ExtensionService) ListItemsByExtension(extensionID string) ([]*ExtensionItemView, error) {
	// Check if extension exists
	if s.registry.GetExtension(extensionID) == nil {
		return nil, fmt.Errorf("extension not found: %s", extensionID)
	}

	items := s.registry.ListItems(extensionID)
	if items == nil {
		return []*ExtensionItemView{}, nil
	}

	views := make([]*ExtensionItemView, 0, len(items))
	for _, item := range items {
		view, err := s.buildItemView(item)
		if err != nil {
			return nil, fmt.Errorf("failed to build view for %s: %w", item.ID, err)
		}
		views = append(views, view)
	}

	return views, nil
}

// Close closes the store
func (s *ExtensionService) Close() error {
	return s.store.Close()
}

// buildExtensionView builds an ExtensionView by merging registry and store data
func (s *ExtensionService) buildExtensionView(ext *Extension) (*ExtensionView, error) {
	view := &ExtensionView{
		Extension: ext,
		Enabled:   true, // default
		Order:     0,    // default
		Items:     []ExtensionItemView{},
	}

	// Try to load config from store
	config, err := s.store.GetConfig(ext.ID)
	if err == nil {
		view.Enabled = config.Enabled
		view.Order = config.Order
	}

	// Build item views
	items := s.registry.ListItems(ext.ID)
	if items != nil {
		for _, item := range items {
			itemView, err := s.buildItemView(item)
			if err != nil {
				return nil, err
			}
			view.Items = append(view.Items, *itemView)
		}
	}

	return view, nil
}

// buildItemView builds an ExtensionItemView by merging registry and store data
func (s *ExtensionService) buildItemView(item *ExtensionItem) (*ExtensionItemView, error) {
	view := &ExtensionItemView{
		ExtensionItem: item,
		Enabled:       true, // default
		Config:        "",   // default
		Order:         0,    // default
	}

	// Try to load config from store
	config, err := s.store.GetConfig(item.ID)
	if err == nil {
		view.Enabled = config.Enabled
		view.Config = config.Config
		view.Order = config.Order
	}

	return view, nil
}
