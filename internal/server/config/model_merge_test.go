package config

import "testing"

func TestMergeModelLists(t *testing.T) {
	tests := []struct {
		name           string
		apiModels      []string
		fallbackModels []string
		want           []string
	}{
		{
			name:           "empty fallback returns api list untouched",
			apiModels:      []string{"gpt-4", "gpt-3.5-turbo"},
			fallbackModels: nil,
			want:           []string{"gpt-4", "gpt-3.5-turbo"},
		},
		{
			name:           "empty api list falls back entirely",
			apiModels:      nil,
			fallbackModels: []string{"gpt-4", "gpt-3.5-turbo"},
			want:           []string{"gpt-4", "gpt-3.5-turbo"},
		},
		{
			name:           "incomplete upstream list gains preset models it omitted",
			apiModels:      []string{"gpt-4"},
			fallbackModels: []string{"gpt-4", "gpt-3.5-turbo", "gpt-4o"},
			want:           []string{"gpt-4", "gpt-3.5-turbo", "gpt-4o"},
		},
		{
			name:           "no duplicates when lists fully overlap",
			apiModels:      []string{"gpt-4", "gpt-3.5-turbo"},
			fallbackModels: []string{"gpt-4", "gpt-3.5-turbo"},
			want:           []string{"gpt-4", "gpt-3.5-turbo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeModelLists(tt.apiModels, tt.fallbackModels)
			if len(got) != len(tt.want) {
				t.Fatalf("MergeModelLists() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("MergeModelLists() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
