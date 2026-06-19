package memory_test

import (
	"testing"

	"github.com/tingly-dev/tingly-box/pkg/memory"
)

func TestByteBufferPool_Copy(t *testing.T) {
	pool := memory.NewByteBufferPool(32, 128)

	tests := []struct {
		name    string
		input   []byte
		wantLen int
	}{
		{
			name:    "empty",
			input:   []byte{},
			wantLen: 0,
		},
		{
			name:    "small",
			input:   []byte("hello"),
			wantLen: 5,
		},
		{
			name:    "large",
			input:   make([]byte, 200),
			wantLen: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pool.Copy(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("Copy() len = %v, want %v", len(got), tt.wantLen)
			}
			if !equal(got, tt.input) {
				t.Errorf("Copy() content mismatch")
			}
		})
	}
}

func TestByteBufferPool_CopyIndependence(t *testing.T) {
	pool := memory.NewByteBufferPool(32, 128)

	// Copy some data
	original := []byte("test data")
	copy1 := pool.Copy(original)
	copy2 := pool.Copy(original)

	// Modify copy1
	copy1[0] = 'X'

	// copy2 should be unchanged
	if copy2[0] != 't' {
		t.Errorf("Copy() returned dependent buffers")
	}

	// original should be unchanged
	if original[0] != 't' {
		t.Errorf("Copy() modified original")
	}
}

func TestDefaultByteBufferPool(t *testing.T) {
	data := []byte("test")
	result := memory.CopyRequestBody(data)

	if len(result) != len(data) {
		t.Errorf("CopyRequestBody() length mismatch")
	}
	if !equal(result, data) {
		t.Errorf("CopyRequestBody() content mismatch")
	}
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
