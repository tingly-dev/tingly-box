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
			name:     "appends port when absent",
			args:     []string{"restart", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"restart", "--daemon", "--port", "9000"},
		},
		{
			name:     "replaces existing --port value form",
			args:     []string{"start", "--port", "8080", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"start", "--daemon", "--port", "9000"},
		},
		{
			name:     "replaces existing --port=value form",
			args:     []string{"start", "--port=8080", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"start", "--daemon", "--port", "9000"},
		},
		{
			name:     "replaces short -p form",
			args:     []string{"start", "-p", "8080", "--daemon"},
			override: []string{"--port", "9000"},
			want:     []string{"start", "--daemon", "--port", "9000"},
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
