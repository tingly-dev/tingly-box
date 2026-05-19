package command

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// withSilencedStdout captures and discards everything written to os.Stdout
// while fn runs. The CLI helpers under test print directly, so tests that
// don't care about the printed output use this to keep test logs clean.
func withSilencedStdout(t *testing.T, fn func()) {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	defer func() {
		_ = w.Close()
		os.Stdout = oldStdout
		<-done
	}()
	fn()
}

// newTestAppManager builds an AppManager with a throwaway config directory.
func newTestAppManager(t *testing.T) *AppManager {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "tingly-test-provider-cli-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	am, err := NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("NewAppManager failed: %v", err)
	}
	return am
}

// TestRunProviderGetByUUID verifies that runProviderGet looks providers up
// strictly by UUID. Names are not unique (UUID is the PK), so a successful
// "lookup by name" would re-introduce the ambiguity bug the previous fix
// closed.
func TestRunProviderGetByUUID(t *testing.T) {
	am := newTestAppManager(t)

	uuid, err := am.AddProvider("my-provider", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	t.Run("known UUID resolves", func(t *testing.T) {
		withSilencedStdout(t, func() {
			if err := runProviderGet(am, uuid); err != nil {
				t.Errorf("runProviderGet(uuid) returned error: %v", err)
			}
		})
	})

	t.Run("name is not accepted as a lookup key", func(t *testing.T) {
		var err error
		withSilencedStdout(t, func() {
			err = runProviderGet(am, "my-provider")
		})
		if err == nil {
			t.Error("runProviderGet(name) should error — names are not the PK")
		}
	})

	t.Run("unknown UUID returns error", func(t *testing.T) {
		var err error
		withSilencedStdout(t, func() {
			err = runProviderGet(am, "00000000-0000-0000-0000-000000000000")
		})
		if err == nil {
			t.Error("expected error for unknown UUID, got nil")
		}
	})
}

// TestRunProviderListDisplaysUUID verifies the list output includes each
// provider's UUID. Operators need the UUID to pass to `config provider get`,
// so hiding it would defeat the lookup-by-UUID design.
func TestRunProviderListDisplaysUUID(t *testing.T) {
	am := newTestAppManager(t)

	uuid, err := am.AddProvider("listed", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdout = w

	listErr := runProviderList(am)

	_ = w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)

	if listErr != nil {
		t.Fatalf("runProviderList returned error: %v", listErr)
	}
	if !strings.Contains(string(out), uuid) {
		t.Errorf("list output should include UUID %q; got:\n%s", uuid, out)
	}
}

// withControlledStdin replaces os.Stdin with a pipe and gives fn a writer it
// can use to feed input on demand. Each runAdd creates its own
// bufio.NewReader(os.Stdin), so the caller must avoid letting one reader
// gulp input intended for the next — write each prompt response only after
// the prior call finishes.
func withControlledStdin(t *testing.T, fn func(stdin io.Writer)) {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	os.Stdin = r
	defer func() {
		_ = w.Close()
		os.Stdin = oldStdin
		_ = r.Close()
	}()
	fn(w)
}

// TestRunAddAllowsDuplicateNames is the regression for the (now-reverted)
// duplicate-name rejection. Two providers with the same display name must
// both be acceptable because the system disambiguates them by UUID.
func TestRunAddAllowsDuplicateNames(t *testing.T) {
	am := newTestAppManager(t)

	args := []string{"dup", "https://api.example.com", "tok", "openai"}

	// runAdd with 4 positional args still calls addProviderWithConfirmation,
	// which prompts y/N. Write each "y" only after the prior call returns
	// so each invocation's freshly-allocated bufio reader sees its prompt.
	withControlledStdin(t, func(stdin io.Writer) {
		withSilencedStdout(t, func() {
			_, _ = io.WriteString(stdin, "y\n")
			if err := runAdd(am, args); err != nil {
				t.Fatalf("first runAdd failed: %v", err)
			}
			_, _ = io.WriteString(stdin, "y\n")
			if err := runAdd(am, args); err != nil {
				t.Fatalf("second runAdd with the same name should succeed, got: %v", err)
			}
		})
	})

	count := 0
	for _, p := range am.ListProviders() {
		if p.Name == "dup" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 providers named %q, got %d", "dup", count)
	}
}

// TestRunAddRejectsInvalidAPIStyle locks down the only validation runAdd
// performs on positional input — anything other than openai/anthropic must
// fail loud rather than silently defaulting.
func TestRunAddRejectsInvalidAPIStyle(t *testing.T) {
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = runAdd(am, []string{"p", "https://api.example.com", "tok", "bogus-style"})
	})
	if err == nil {
		t.Fatal("expected error for invalid API style, got nil")
	}
}

// TestConfigProviderGetCmdKongUsesUUID is a structural assertion: the field
// the user supplies on the command line must be named UUID (not Name), so
// the help text and behavior stay consistent with "providers are keyed by
// UUID". A regression here would silently rename the positional arg back
// to a name.
func TestConfigProviderGetCmdKongUsesUUID(t *testing.T) {
	cfg := ConfigProviderGetCmdKong{UUID: "abc"}
	if cfg.UUID != "abc" {
		t.Errorf("ConfigProviderGetCmdKong.UUID round-trip failed: got %q", cfg.UUID)
	}
}

// TestConfigProviderGetCmdKongRunWithUUID verifies Run forwards a supplied
// UUID to runProviderGet (rather than dropping into interactive mode).
func TestConfigProviderGetCmdKongRunWithUUID(t *testing.T) {
	am := newTestAppManager(t)

	uuid, err := am.AddProvider("p", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	cfg := ConfigProviderGetCmdKong{UUID: uuid}
	withSilencedStdout(t, func() {
		if err := cfg.Run(am); err != nil {
			t.Errorf("ConfigProviderGetCmdKong.Run with valid UUID returned error: %v", err)
		}
	})
}
