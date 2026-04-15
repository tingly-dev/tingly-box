package extension

import (
	"fmt"
	"sync"
)

// ExtensionRegistry manages code-defined extensions and items in memory
type ExtensionRegistry struct {
	extensions map[string]*Extension
	items      map[string]map[string]*ExtensionItem // extensionID -> itemID -> item
	mu         sync.RWMutex
}

// NewExtensionRegistry creates a new extension registry
func NewExtensionRegistry() *ExtensionRegistry {
	return &ExtensionRegistry{
		extensions: make(map[string]*Extension),
		items:      make(map[string]map[string]*ExtensionItem),
	}
}

// RegisterExtension registers a new extension
func (r *ExtensionRegistry) RegisterExtension(ext *Extension) error {
	if ext == nil {
		return fmt.Errorf("extension cannot be nil")
	}
	if ext.ID == "" {
		return fmt.Errorf("extension ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.extensions[ext.ID]; exists {
		return fmt.Errorf("extension already registered: %s", ext.ID)
	}

	r.extensions[ext.ID] = ext
	// Initialize items map for this extension
	if r.items[ext.ID] == nil {
		r.items[ext.ID] = make(map[string]*ExtensionItem)
	}

	return nil
}

// RegisterItem registers a new item for an extension
func (r *ExtensionRegistry) RegisterItem(item *ExtensionItem) error {
	if item == nil {
		return fmt.Errorf("item cannot be nil")
	}
	if item.ID == "" {
		return fmt.Errorf("item ID cannot be empty")
	}
	if item.ExtensionID == "" {
		return fmt.Errorf("item ExtensionID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if extension exists
	if _, exists := r.extensions[item.ExtensionID]; !exists {
		return fmt.Errorf("extension not found: %s", item.ExtensionID)
	}

	// Check if item already exists
	if r.items[item.ExtensionID] != nil {
		if _, exists := r.items[item.ExtensionID][item.ID]; exists {
			return fmt.Errorf("item already registered: %s/%s", item.ExtensionID, item.ID)
		}
	} else {
		r.items[item.ExtensionID] = make(map[string]*ExtensionItem)
	}

	r.items[item.ExtensionID][item.ID] = item
	return nil
}

// GetExtension retrieves an extension by ID
func (r *ExtensionRegistry) GetExtension(id string) *Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.extensions[id]
}

// GetItem retrieves an item by extension ID and item ID
func (r *ExtensionRegistry) GetItem(extensionID, itemID string) *ExtensionItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.items[extensionID] == nil {
		return nil
	}
	return r.items[extensionID][itemID]
}

// ListExtensions returns all registered extensions
func (r *ExtensionRegistry) ListExtensions() []*Extension {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extensions := make([]*Extension, 0, len(r.extensions))
	for _, ext := range r.extensions {
		extensions = append(extensions, ext)
	}
	return extensions
}

// ListItems returns all items for a specific extension
func (r *ExtensionRegistry) ListItems(extensionID string) []*ExtensionItem {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.items[extensionID] == nil {
		return nil
	}

	items := make([]*ExtensionItem, 0, len(r.items[extensionID]))
	for _, item := range r.items[extensionID] {
		items = append(items, item)
	}
	return items
}
