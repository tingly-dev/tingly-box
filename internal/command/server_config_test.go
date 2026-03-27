package command

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestResolveConfigDirFromCmd(t *testing.T) {
	tests := []struct {
		name             string
		localConfigDir   string
		defaultConfigDir string
		expected         string
		expectError      bool
	}{
		{
			name:             "only default specified",
			localConfigDir:   "",
			defaultConfigDir: "/default/path",
			expected:         "/default/path",
			expectError:      false,
		},
		{
			name:             "only local specified",
			localConfigDir:   "/local/path",
			defaultConfigDir: "",
			expected:         "/local/path",
			expectError:      false,
		},
		{
			name:             "local overrides default",
			localConfigDir:   "/local/path",
			defaultConfigDir: "/default/path",
			expected:         "/local/path",
			expectError:      false,
		},
		{
			name:             "local and default same",
			localConfigDir:   "/same/path",
			defaultConfigDir: "/same/path",
			expected:         "/same/path",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			if tt.localConfigDir != "" {
				cmd.Flags().String("config-dir", tt.localConfigDir, "")
			} else {
				cmd.Flags().String("config-dir", "", "")
			}

			result, err := resolveConfigDirFromCmd(cmd, tt.defaultConfigDir)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
