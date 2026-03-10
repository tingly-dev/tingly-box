// Package jsonstore provides a generic JSON file-based key-value storage.
// It is designed for simple persistence needs where a full database is overkill.
//
// Features:
//   - Thread-safe operations with read-write mutexes
//   - Atomic file writes (write to temp, then rename)
//   - In-memory caching for fast reads
//   - Dirty flag to only write when data changes
//   - Generic model support through type parameters
//
// Example usage:
//
//	type MyModel struct {
//	    Name  string `json:"name"`
//	    Count int    `json:"count"`
//	}
//
//	store, err := jsonstore.New[MyModel]("data.json")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	// Set a value
//	err = store.Set("key1", &MyModel{Name: "test", Count: 42})
//
//	// Get a value
//	model, ok := store.Get("key1")
package jsonstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Model is the interface that all models must implement.
// Use a pointer to your model type as the type parameter.
type Model interface {
	any // Allows any type as a model
}

// StoreData represents the JSON file structure.
// Version allows for future schema migrations.
type StoreData[T any] struct {
	Version int           `json:"version"`
	Items   map[string]*T `json:"items"` // Key: string, Value: pointer to model
	Updated time.Time     `json:"updated"`
}

// Store manages JSON file-based storage with generic type support.
type Store[T any] struct {
	mu       sync.RWMutex
	filePath string
	data     *StoreData[T]
	dirty    bool // Track if data needs to be written
}

// StoreOption is a function that configures a Store.
type StoreOption[T any] func(*Store[T])

// WithVersion sets the version for the store data.
func WithVersion[T any](version int) StoreOption[T] {
	return func(s *Store[T]) {
		s.data.Version = version
	}
}

// New creates a new JSON-based store for the given model type.
// If filePath doesn't exist, an empty store is created.
// If filePath exists, existing data is loaded.
//
// The model type T must be a pointer type (e.g., *MyModel) to ensure
// consistent behavior when values are nil.
func New[T any](filePath string, opts ...StoreOption[T]) (*Store[T], error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path is required")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	store := &Store[T]{
		filePath: filePath,
		data: &StoreData[T]{
			Version: 1,
			Items:   make(map[string]*T),
			Updated: time.Now().UTC(),
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(store)
	}

	// Try to load existing data
	if err := store.load(); err != nil {
		// If file doesn't exist, that's ok - we start fresh
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load store: %w", err)
		}
	}

	return store, nil
}

// load reads the JSON file into memory.
func (s *Store[T]) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if file exists
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		s.data.Items = make(map[string]*T)
		s.data.Updated = time.Now().UTC()
		s.dirty = false
		return nil
	}

	// Read file
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var storeData StoreData[T]
	if err := json.Unmarshal(data, &storeData); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate version
	if storeData.Version > s.data.Version {
		return fmt.Errorf("unsupported store version: %d (max: %d)", storeData.Version, s.data.Version)
	}

	// Ensure items map is initialized
	if storeData.Items == nil {
		storeData.Items = make(map[string]*T)
	}

	s.data = &storeData
	s.dirty = false
	return nil
}

// save writes the current data to disk atomically.
func (s *Store[T]) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil // No changes to save
	}

	// Update timestamp
	s.data.Updated = time.Now().UTC()

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to temporary file
	tmpPath := s.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		// Clean up temp file on error
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	s.dirty = false
	return nil
}

// Close ensures data is persisted before closing.
func (s *Store[T]) Close() error {
	return s.save()
}

// Get retrieves a value by key. Returns the value and true if found, nil and false otherwise.
func (s *Store[T]) Get(key string) *T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Items == nil {
		return nil
	}

	value, ok := s.data.Items[key]
	if !ok {
		return nil
	}

	// Return the pointer directly - callers should not modify the returned value
	// If modification is needed, callers should make their own copy
	return value
}

// Has checks if a key exists in the store.
func (s *Store[T]) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Items == nil {
		return false
	}
	_, ok := s.data.Items[key]
	return ok
}

// Set stores a value for the given key.
// The value is stored as a pointer to enable nil values.
func (s *Store[T]) Set(key string, value *T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Items[key] = value
	s.dirty = true

	return nil
}

// Delete removes a value from the store.
// Returns true if the key was found and removed, false otherwise.
func (s *Store[T]) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.Items == nil {
		return false
	}

	if _, ok := s.data.Items[key]; !ok {
		return false
	}

	delete(s.data.Items, key)
	s.dirty = true
	return true
}

// List returns all values in the store as a map.
// The returned map is a shallow copy; modifications won't affect the store.
func (s *Store[T]) List() map[string]*T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Items == nil {
		return make(map[string]*T)
	}

	// Return a copy to avoid race conditions
	result := make(map[string]*T, len(s.data.Items))
	for key, value := range s.data.Items {
		result[key] = value
	}
	return result
}

// Keys returns all keys in the store.
func (s *Store[T]) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Items == nil {
		return nil
	}

	keys := make([]string, 0, len(s.data.Items))
	for key := range s.data.Items {
		keys = append(keys, key)
	}
	return keys
}

// Count returns the number of items in the store.
func (s *Store[T]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Items == nil {
		return 0
	}
	return len(s.data.Items)
}

// Clear removes all items from the store.
func (s *Store[T]) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Items = make(map[string]*T)
	s.data.Updated = time.Now().UTC()
	s.dirty = true

	return nil
}

// Update updates a value using the provided function.
// The function receives the current value (or nil if not found) and returns the new value.
// If the function returns nil, the key is deleted.
func (s *Store[T]) Update(key string, fn func(*T) *T) error {
	if fn == nil {
		return fmt.Errorf("update function is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.data.Items[key]
	newValue := fn(current)

	if newValue == nil {
		// Delete the key
		if current != nil {
			delete(s.data.Items, key)
			s.dirty = true
		}
		return nil
	}

	// Update or set the value
	s.data.Items[key] = newValue
	s.dirty = true

	return nil
}

// GetVersion returns the store version.
func (s *Store[T]) GetVersion() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Version
}

// GetUpdated returns the last update time.
func (s *Store[T]) GetUpdated() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Updated
}

// ForceSave saves the data even if not marked as dirty.
func (s *Store[T]) ForceSave() error {
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	return s.save()
}
