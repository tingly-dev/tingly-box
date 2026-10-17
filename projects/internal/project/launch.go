package project

import (
	"context"
	"errors"
	"runtime"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Optional Host capabilities for folder sessions. A host implements them on
// the same value it passes as Host; Projects type-asserts, so a host without
// them (the standalone binary) keeps compiling and gets the generic behaviour.

// Launcher routes a folder session's turns by a profile the host names
// (Tingly Box: a Claude Code profile). Projects treats profiles as opaque.
type Launcher interface {
	// LaunchEnv is the environment and settings file a session's turn runs
	// Claude Code with under profile ("" for the host's default routing).
	LaunchEnv(ctx context.Context, profile string) (env []string, settingsPath string, err error)
	// CheckLaunch rejects a profile and model the host cannot run, so the
	// mistake shows on the request rather than as a fallback on the next turn.
	CheckLaunch(ctx context.Context, profile, model string) error
}

// Handoffer names the program that starts Claude Code in a terminal the way
// the host's turns do under profile, when that is not plain `claude`. Projects
// adds the session, its model and the folder, quoted for the user's shell.
type Handoffer interface {
	HandoffLaunch(profile string) []string
}

// Errors for launch settings and handoff.
var (
	ErrNoProfiles = errors.New("this server has no launch profiles")
	ErrBusy       = errors.New("stop or wait for the current turn first")
)

// SetProfile sets the profile the session's next turn launches with. A model
// the new profile cannot run is cleared rather than refused.
func (m *Threads) SetProfile(ctx context.Context, id, profile string) (*db.Thread, error) {
	profile = strings.TrimSpace(profile)
	t, err := m.store.GetSession(id)
	if err != nil {
		return nil, err
	}
	model := t.Model
	if profile != "" {
		l, ok := m.env.(Launcher)
		if !ok {
			return nil, ErrNoProfiles
		}
		if err := l.CheckLaunch(ctx, profile, ""); err != nil {
			return nil, err
		}
		if model != "" && l.CheckLaunch(ctx, profile, model) != nil {
			model = ""
		}
	}
	out, err := m.update(id, func(t *db.Thread) { t.Profile, t.Model = profile, model })
	if err == nil {
		m.addEvent(out, &db.ThreadEvent{Kind: db.EvSystem, Content: "profile: " + orDefault(profile)})
	}
	return out, err
}

// SetModel sets the model the session's next turn asks for ("" for the
// profile's default).
func (m *Threads) SetModel(ctx context.Context, id, model string) (*db.Thread, error) {
	model = strings.TrimSpace(model)
	t, err := m.store.GetSession(id)
	if err != nil {
		return nil, err
	}
	if l, ok := m.env.(Launcher); ok && model != "" {
		if err := l.CheckLaunch(ctx, t.Profile, model); err != nil {
			return nil, err
		}
	}
	out, err := m.update(id, func(t *db.Thread) { t.Model = model })
	if err == nil {
		m.addEvent(out, &db.ThreadEvent{Kind: db.EvSystem, Content: "model: " + orDefault(model)})
	}
	return out, err
}

// Handoff releases a session so it can go on in a terminal, and returns the
// command that does it. The resident process is closed first: two processes
// writing one Claude session file corrupt it. A turn in flight is refused
// rather than killed; a message sent here afterwards starts it again.
func (m *Threads) Handoff(id string) (string, error) {
	// A message sent while the process was closing waited for it.
	defer m.next(id)
	t, err := m.store.GetSession(id)
	if err != nil {
		return "", err
	}
	if m.Running(id) {
		return "", ErrBusy
	}
	if err := m.engine.Release(m.sessionOf(id)); errors.Is(err, engine.ErrBusy) {
		return "", ErrBusy
	} else if err != nil {
		return "", err
	}
	argv := []string{"claude"}
	if h, ok := m.env.(Handoffer); ok {
		argv = h.HandoffLaunch(t.Profile)
	}
	argv = append(argv, "--resume", t.SessionID)
	if t.Model != "" {
		argv = append(argv, "--model", t.Model)
	}
	return terminalCommand(runtime.GOOS, t.Worktree, argv), nil
}

// terminalCommand runs argv in dir, written for the shell a person pastes
// into: PowerShell on Windows, a POSIX shell elsewhere.
func terminalCommand(goos, dir string, argv []string) string {
	words := make([]string, len(argv))
	if goos == "windows" {
		for i, a := range argv {
			words[i] = powerShellQuote(a)
		}
		return "Set-Location -LiteralPath " + powerShellQuote(dir) + "; & " + strings.Join(words, " ")
	}
	for i, a := range argv {
		words[i] = shellWord(a)
	}
	return "cd " + ShellQuote(dir) + " && " + strings.Join(words, " ")
}

// ShellQuote single-quotes s for a POSIX shell.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellWord leaves plain words (flags, subcommands) bare and quotes the rest.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:=@") == "" {
		return s
	}
	return ShellQuote(s)
}

// powerShellQuote single-quotes s for PowerShell, where a quote doubles.
func powerShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func orDefault(s string) string {
	if s == "" {
		return "default"
	}
	return s
}
