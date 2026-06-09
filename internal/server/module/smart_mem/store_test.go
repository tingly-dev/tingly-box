package smart_mem

import (
	"errors"
	"strings"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	id := "4f9c5b2a-8d11-4c2c-9f3b-1e7c6d0f0a02"
	payload := []byte(`{"hello":"world"}`)

	if err := store.Put(id, payload); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("round trip mismatch: got %q want %q", got, payload)
	}
}

func TestFileStoreNotFound(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	_, err = store.Get("4f9c5b2a-8d11-4c2c-9f3b-1e7c6d0f0a02")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreRejectsBadUUID(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, err := store.Get("../etc/passwd"); err == nil {
		t.Fatalf("expected error for path traversal key")
	}
}

func TestDeriveDescriptionTruncates(t *testing.T) {
	raw := []byte(`{"a":"` + strings.Repeat("x", 500) + `"}`)
	desc := deriveDescription(raw)
	if !strings.HasSuffix(desc, "...") {
		t.Fatalf("expected truncation suffix, got %q", desc)
	}
	if len([]rune(desc)) > descriptionMaxRunes+3 {
		t.Fatalf("description too long: %d runes", len([]rune(desc)))
	}
}

func TestDeriveDescriptionCollapsesWhitespace(t *testing.T) {
	raw := []byte("{\n  \"k\":   \"v\"\n}")
	got := deriveDescription(raw)
	want := `{ "k": "v" }`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
