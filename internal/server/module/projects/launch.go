package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
)

// Folder sessions run through Tingly Box's Claude Code
// profiles, pick a model tier, and hand off to `tingly-box cc --resume`.
// Projects sees a profile only as a name; these are the Tingly Box halves of
// its optional Launcher and Handoffer.

var (
	_ projectsapp.Launcher  = (*Host)(nil)
	_ projectsapp.Handoffer = (*Host)(nil)
)

// LaunchEnv routes a session's turn like @cc does: the main
// claude_code scenario's env, or a profile's settings file, which carries
// its own routing and so replaces the env rather than adding to it.
func (h *Host) LaunchEnv(ctx context.Context, profile string) ([]string, string, error) {
	if profile == "" {
		env, err := h.cc.GetClaudeCodeEnv(ctx)
		return env, "", err
	}
	path, err := h.cc.GetClaudeCodeSettingsPathForProfile(ctx, profile)
	return nil, path, err
}

// CheckLaunch accepts the default tier always, and a named tier only where
// the profile routes tiers separately: under a unified profile every tier is
// the same model, so choosing one would change nothing.
func (h *Host) CheckLaunch(ctx context.Context, profile, model string) error {
	env, err := h.claudeEnv(ctx, profile)
	if err != nil {
		return fmt.Errorf("profile %q: %w", profile, err)
	}
	if model == "" {
		return nil
	}
	if !slices.Contains(agent.ClaudeCodeTierAliases, model) {
		return fmt.Errorf("unknown model tier %q", model)
	}
	if agent.ClaudeCodeTiersFromEnv(env).Unified {
		return errors.New("this profile routes every tier to one model; edit its rules to change it")
	}
	return nil
}

// claudeEnv is the env Claude Code is given under profile, so its tiers are
// exactly what the process will request.
func (h *Host) claudeEnv(ctx context.Context, profile string) (map[string]string, error) {
	if profile == "" {
		list, err := h.cc.GetClaudeCodeEnv(ctx)
		if err != nil {
			return nil, err
		}
		env := make(map[string]string, len(list))
		for _, kv := range list {
			if k, v, ok := strings.Cut(kv, "="); ok {
				env[k] = v
			}
		}
		return env, nil
	}
	path, err := h.cc.GetClaudeCodeSettingsPathForProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	return agent.ReadClaudeCodeSettingsEnv(path)
}

// HandoffLaunch goes through tingly-box (`cc`, or `profile <id>`), by
// absolute path, so the terminal routes through the same gateway and
// settings the web turns used, with no token in the command.
func (h *Host) HandoffLaunch(profile string) []string {
	if profile != "" {
		return []string{selfExecutable(), "profile", profile}
	}
	return []string{selfExecutable(), "cc"}
}

// selfExecutable works whether tingly-box is on PATH, run through npx, or
// started from a build directory.
func selfExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return "tingly-box"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}
