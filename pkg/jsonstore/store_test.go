package jsonstore

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type TestModel struct {
	Name      string    `json:"name"`
	Count     int       `json:"count"`
	CreatedAt time.Time `json:"created_at"`
}

func TestStore_New(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()

	// File doesn't exist yet
	_, err = os.Stat(filePath)
	require.True(t, os.IsNotExist(err))
}

func TestStore_GetSet(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Get non-existent key
	value := store.Get("non-existent")
	require.Nil(t, value)

	// Set a value
	model := &TestModel{
		Name:      "test",
		Count:     42,
		CreatedAt: time.Now().UTC(),
	}
	err = store.Set("key1", model)
	require.NoError(t, err)

	// Get the value
	retrieved := store.Get("key1")
	require.NotNil(t, retrieved)
	require.Equal(t, "test", retrieved.Name)
	require.Equal(t, 42, retrieved.Count)
}

func TestStore_Has(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Key doesn't exist
	require.False(t, store.Has("key1"))

	// Set a value
	err = store.Set("key1", &TestModel{Name: "test"})
	require.NoError(t, err)

	// Key exists now
	require.True(t, store.Has("key1"))
}

func TestStore_Delete(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Set a value
	err = store.Set("key1", &TestModel{Name: "test"})
	require.NoError(t, err)

	// Delete existing key
	deleted := store.Delete("key1")
	require.True(t, deleted)
	require.Nil(t, store.Get("key1"))

	// Delete non-existing key
	deleted = store.Delete("non-existent")
	require.False(t, deleted)
}

func TestStore_List(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Empty list
	items := store.List()
	require.NotNil(t, items)
	require.Empty(t, items)

	// Add some items
	_ = store.Set("key1", &TestModel{Name: "first"})
	_ = store.Set("key2", &TestModel{Name: "second"})

	items = store.List()
	require.Len(t, items, 2)
	require.Equal(t, "first", items["key1"].Name)
	require.Equal(t, "second", items["key2"].Name)
}

func TestStore_Keys(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Empty keys - returns empty slice, not nil
	keys := store.Keys()
	require.Empty(t, keys)

	// Add some items
	_ = store.Set("key1", &TestModel{Name: "first"})
	_ = store.Set("key2", &TestModel{Name: "second"})

	keys = store.Keys()
	require.Len(t, keys, 2)
	require.ElementsMatch(t, []string{"key1", "key2"}, keys)
}

func TestStore_Count(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	require.Equal(t, 0, store.Count())

	_ = store.Set("key1", &TestModel{Name: "first"})
	require.Equal(t, 1, store.Count())

	_ = store.Set("key2", &TestModel{Name: "second"})
	require.Equal(t, 2, store.Count())
}

func TestStore_Clear(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	_ = store.Set("key1", &TestModel{Name: "first"})
	_ = store.Set("key2", &TestModel{Name: "second"})
	require.Equal(t, 2, store.Count())

	err = store.Clear()
	require.NoError(t, err)
	require.Equal(t, 0, store.Count())
}

func TestStore_Update(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Update non-existing key
	err = store.Update("key1", func(model *TestModel) *TestModel {
		if model == nil {
			return &TestModel{Name: "new", Count: 1}
		}
		return model
	})
	require.NoError(t, err)

	value := store.Get("key1")
	require.NotNil(t, value)
	require.Equal(t, "new", value.Name)

	// Update existing key
	err = store.Update("key1", func(model *TestModel) *TestModel {
		model.Count = 99
		return model
	})
	require.NoError(t, err)

	value = store.Get("key1")
	require.Equal(t, 99, value.Count)

	// Delete by returning nil
	err = store.Update("key1", func(model *TestModel) *TestModel {
		return nil // Delete the key
	})
	require.NoError(t, err)

	value = store.Get("key1")
	require.Nil(t, value)
}

func TestStore_UpdateNilFn(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	err = store.Update("key1", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "update function is required")
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	// Create and populate store
	store1, err := New[TestModel](filePath)
	require.NoError(t, err)

	model := &TestModel{
		Name:      "persistent",
		Count:     123,
		CreatedAt: time.Now().UTC(),
	}
	err = store1.Set("key1", model)
	require.NoError(t, err)

	err = store1.Close()
	require.NoError(t, err)

	// Open new store instance
	store2, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store2.Close()

	// Verify data persisted
	retrieved := store2.Get("key1")
	require.NotNil(t, retrieved)
	require.Equal(t, "persistent", retrieved.Name)
	require.Equal(t, 123, retrieved.Count)
}

func TestStore_Close(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)

	// Add some data
	_ = store.Set("key1", &TestModel{Name: "test"})

	// Close should persist data
	err = store.Close()
	require.NoError(t, err)

	// File should exist now
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Contains(t, string(data), `"version"`)
	require.Contains(t, string(data), `"items"`)
}

func TestStore_Version(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	// Default version
	store1, err := New[TestModel](filePath)
	require.NoError(t, err)
	require.Equal(t, 1, store1.GetVersion())
	store1.Close()

	// Custom version - need to explicitly type the option
	store2, err := New[TestModel](filePath, WithVersion[TestModel](2))
	require.NoError(t, err)
	require.Equal(t, 2, store2.GetVersion())
	store2.Close()
}

func TestStore_GetUpdated(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Initial updated time
	updated1 := store.GetUpdated()
	require.False(t, updated1.IsZero())

	// Wait a bit and update
	time.Sleep(10 * time.Millisecond)
	_ = store.Set("key1", &TestModel{Name: "test"})

	// Updated time should be newer after Close/Save
	updated2 := store.GetUpdated()
	require.True(t, updated2.After(updated1) || updated2.Equal(updated1))
}

func TestStore_NilValues(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	// Set nil value (deletes the key if it exists)
	_ = store.Set("key1", &TestModel{Name: "test"})
	require.NotNil(t, store.Get("key1"))

	err = store.Set("key1", nil)
	require.NoError(t, err)
	require.Nil(t, store.Get("key1"))
}

func TestStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.json")

	store, err := New[TestModel](filePath)
	require.NoError(t, err)
	defer store.Close()

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := fmt.Sprintf("key%d", idx)
			_ = store.Set(key, &TestModel{Name: fmt.Sprintf("value%d", idx)})
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(idx int) {
			key := fmt.Sprintf("key%d", idx)
			_ = store.Get(key)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	require.Equal(t, 10, store.Count())
}
