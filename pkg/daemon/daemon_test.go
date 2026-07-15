package daemon

import (
	"reflect"
	"testing"
)

func TestBuildDaemonArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		override []string
		want     []string
	}{
		{
			name:     "no override passes through unchanged",
			args:     []string{"restart", "--daemon"},
			override: nil,
			want:     []string{"restart", "--daemon"},
		},
		{
			name:     "appends pinned port (restart preserve case)",
			args:     []string{"restart", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"restart", "--daemon", "--port", "9000"},
		},
		{
			// An earlier --port is left in place; the CLI parser takes the last
			// occurrence, so the appended value wins without stripping.
			name:     "appends after an existing --port (last wins)",
			args:     []string{"start", "--port", "8080", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"start", "--port", "8080", "--daemon", "--port", "9000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDaemonArgs(tt.args, tt.override)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildDaemonArgs(%v, %v) = %v, want %v", tt.args, tt.override, got, tt.want)
			}
		})
	}
}
