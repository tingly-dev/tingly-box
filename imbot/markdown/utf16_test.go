package markdown

import (
	"testing"
)

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "ASCII text",
			input:    "Hello",
			expected: 5,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "CJK characters",
			input:    "你好",
			expected: 2,
		},
		{
			name:     "Emoji (non-BMP)",
			input:    "👍",
			expected: 2, // Surrogate pair
		},
		{
			name:     "Mixed text with emoji",
			input:    "Hello 👍",
			expected: 8, // 5 + 1 space + 2 for emoji
		},
		{
			name:     "Multiple emojis",
			input:    "👍😀🎉",
			expected: 6, // 3 emojis × 2
		},
		{
			name:     "Mixed CJK and ASCII",
			input:    "Hello你好",
			expected: 7, // 5 ASCII + 2 CJK
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UTF16Len(tt.input)
			if got != tt.expected {
				t.Errorf("UTF16Len(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUTF16OffsetTable(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, offsets []int)
	}{
		{
			name:  "ASCII text",
			input: "Hello",
			check: func(t *testing.T, offsets []int) {
				// Offsets should be [0, 1, 2, 3, 4, 5]
				expected := []int{0, 1, 2, 3, 4, 5}
				for i, exp := range expected {
					if offsets[i] != exp {
						t.Errorf("offset[%d] = %d, want %d", i, offsets[i], exp)
					}
				}
			},
		},
		{
			name:  "Emoji",
			input: "👍",
			check: func(t *testing.T, offsets []int) {
				// Emoji is 4 bytes but 2 UTF-16 units
				if len(offsets) != 5 { // 4 bytes + 1 sentinel
					t.Errorf("len(offsets) = %d, want 5", len(offsets))
				}
				// All bytes of emoji map to offset 0
				for i := 0; i < 4; i++ {
					if offsets[i] != 0 {
						t.Errorf("offset[%d] = %d, want 0", i, offsets[i])
					}
				}
				// Sentinel should be 2
				if offsets[4] != 2 {
					t.Errorf("offset[4] = %d, want 2", offsets[4])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offsets := UTF16OffsetTable(tt.input)
			tt.check(t, offsets)
		})
	}
}
