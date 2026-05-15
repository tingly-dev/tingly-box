package command

import (
	"reflect"
	"testing"
)

func TestParseCCFlags(t *testing.T) {
	cases := []struct {
		name           string
		args           []string
		wantProfile    string
		wantPort       int
		wantClaudeArgs []string
		wantErr        bool
	}{
		{
			name:           "empty args",
			args:           nil,
			wantProfile:    "",
			wantPort:       0,
			wantClaudeArgs: nil,
		},
		{
			name:           "profile long form",
			args:           []string{"--profile", "dev"},
			wantProfile:    "dev",
			wantPort:       0,
			wantClaudeArgs: nil,
		},
		{
			name:           "profile short form",
			args:           []string{"-p", "dev"},
			wantProfile:    "dev",
			wantPort:       0,
			wantClaudeArgs: nil,
		},
		{
			name:           "tingly-port only",
			args:           []string{"--tingly-port", "13580"},
			wantProfile:    "",
			wantPort:       13580,
			wantClaudeArgs: nil,
		},
		{
			name:           "profile and tingly-port",
			args:           []string{"--profile", "dev", "--tingly-port", "13580"},
			wantProfile:    "dev",
			wantPort:       13580,
			wantClaudeArgs: nil,
		},
		{
			name:           "tingly-port before profile",
			args:           []string{"--tingly-port", "13580", "-p", "dev"},
			wantProfile:    "dev",
			wantPort:       13580,
			wantClaudeArgs: nil,
		},
		{
			name:           "passthrough after tingly flags",
			args:           []string{"--profile", "dev", "--tingly-port", "13580", "--resume", "abc"},
			wantProfile:    "dev",
			wantPort:       13580,
			wantClaudeArgs: []string{"--resume", "abc"},
		},
		{
			name:           "unrecognized flag stops scanning",
			args:           []string{"--resume", "abc", "--profile", "dev"},
			wantProfile:    "",
			wantPort:       0,
			wantClaudeArgs: []string{"--resume", "abc", "--profile", "dev"},
		},
		{
			name:    "profile missing value",
			args:    []string{"--profile"},
			wantErr: true,
		},
		{
			name:    "tingly-port missing value",
			args:    []string{"--tingly-port"},
			wantErr: true,
		},
		{
			name:    "tingly-port non-numeric",
			args:    []string{"--tingly-port", "abc"},
			wantErr: true,
		},
		{
			name:    "tingly-port zero",
			args:    []string{"--tingly-port", "0"},
			wantErr: true,
		},
		{
			name:    "tingly-port out of range",
			args:    []string{"--tingly-port", "70000"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile, port, claudeArgs, err := parseCCFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (profile=%q port=%d claudeArgs=%v)", profile, port, claudeArgs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if profile != tc.wantProfile {
				t.Errorf("profile: got %q, want %q", profile, tc.wantProfile)
			}
			if port != tc.wantPort {
				t.Errorf("port: got %d, want %d", port, tc.wantPort)
			}
			if !reflect.DeepEqual(claudeArgs, tc.wantClaudeArgs) {
				t.Errorf("claudeArgs: got %v, want %v", claudeArgs, tc.wantClaudeArgs)
			}
		})
	}
}
