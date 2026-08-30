package command

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
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

	uuid, err := addProviderForTest(am, "my-provider", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
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
// provider's UUID. Operators need the UUID to pass to `provider get`,
// so hiding it would defeat the lookup-by-UUID design.
func TestRunProviderListDisplaysUUID(t *testing.T) {
	am := newTestAppManager(t)

	uuid, err := addProviderForTest(am, "listed", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
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

// TestProviderAdd_AllowsDuplicateNames is the regression for the
// (now-reverted) duplicate-name rejection. Two providers with the same
// display name must both be acceptable because the system disambiguates
// them by UUID.
func TestProviderAdd_AllowsDuplicateNames(t *testing.T) {
	am := newTestAppManager(t)

	cmd := ProviderAddCmdKong{Name: "dup", BaseURL: "https://api.example.com", Token: "tok", APIStyle: "openai"}

	withSilencedStdout(t, func() {
		if err := cmd.Run(am); err != nil {
			t.Fatalf("first add failed: %v", err)
		}
		if err := cmd.Run(am); err != nil {
			t.Fatalf("second add with the same name should succeed, got: %v", err)
		}
	})

	count := 0
	for _, p := range am.GetGlobalConfig().ListProviders() {
		if p.Name == "dup" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 providers named %q, got %d", "dup", count)
	}
}

// TestProviderAdd_RejectsInvalidAPIStyle locks down the only validation Run
// performs beyond "all four given" — anything other than openai/anthropic
// must fail loud rather than silently defaulting.
func TestProviderAdd_RejectsInvalidAPIStyle(t *testing.T) {
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&ProviderAddCmdKong{Name: "p", BaseURL: "https://api.example.com", Token: "tok", APIStyle: "bogus-style"}).Run(am)
	})
	if err == nil {
		t.Fatal("expected error for invalid API style, got nil")
	}
}

// TestProviderGetCmdKongUsesUUID is a structural assertion: the field the
// user supplies on the command line must be named UUID (not Name), so the
// help text and behavior stay consistent with "providers are keyed by
// UUID". A regression here would silently rename the positional arg back
// to a name.
func TestProviderGetCmdKongUsesUUID(t *testing.T) {
	cfg := ProviderGetCmdKong{UUID: "abc"}
	if cfg.UUID != "abc" {
		t.Errorf("ProviderGetCmdKong.UUID round-trip failed: got %q", cfg.UUID)
	}
}

// TestProviderGetCmdKongRunWithUUID verifies Run forwards a supplied UUID to
// runProviderGet.
func TestProviderGetCmdKongRunWithUUID(t *testing.T) {
	am := newTestAppManager(t)

	uuid, err := addProviderForTest(am, "p", "https://api.example.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}

	cfg := ProviderGetCmdKong{UUID: uuid}
	withSilencedStdout(t, func() {
		if err := cfg.Run(am); err != nil {
			t.Errorf("ProviderGetCmdKong.Run with valid UUID returned error: %v", err)
		}
	})
}

// provider add/update/delete are a pure CI surface: every operation is a
// single, fully-parameterized invocation with no interactive fallback of
// any kind (not even TTY-gated) — picking something is `tingly-box tui` or
// Web UI work. These tests guard that shape directly, not via a TTY check.

func TestProviderAdd_PartialArgs_ClearError(t *testing.T) {
	cases := []ProviderAddCmdKong{
		{},
		{Name: "openai"},
		{Name: "openai", BaseURL: "https://api.openai.com"},
		{Name: "openai", BaseURL: "https://api.openai.com", Token: "tok"},
	}
	for _, cmd := range cases {
		err := cmd.Run(nil)
		if err == nil {
			t.Fatalf("%+v: expected an error, got nil", cmd)
		}
		if !strings.Contains(err.Error(), "required") {
			t.Errorf("%+v: error = %q, want it to say all four are required", cmd, err.Error())
		}
	}
}

func TestProviderAdd_AllFourArgs_Works(t *testing.T) {
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&ProviderAddCmdKong{
			Name: "openai", BaseURL: "https://api.openai.com", Token: "tok", APIStyle: "openai",
		}).Run(am)
	})
	if err != nil {
		t.Errorf("expected success with all four args given, got: %v", err)
	}
}

func TestProviderUpdate_NothingToUpdate_ClearError(t *testing.T) {
	err := (&ProviderUpdateCmdKong{UUID: "some-uuid"}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("error = %q, want it to say there's nothing to update", err.Error())
	}
}

func TestProviderUpdate_ChangesOnlyGivenFields(t *testing.T) {
	am := newTestAppManager(t)
	uuid, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("addProviderForTest: %v", err)
	}

	withSilencedStdout(t, func() {
		err = (&ProviderUpdateCmdKong{UUID: uuid, Name: "renamed"}).Run(am)
	})
	if err != nil {
		t.Fatalf("ProviderUpdateCmdKong.Run: %v", err)
	}

	getResult, getErr := usecase.NewProviderUseCase(am.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: uuid})
	if getErr != nil {
		t.Fatalf("get after update: %v", getErr)
	}
	got := getResult.Provider
	if got.Name != "renamed" {
		t.Errorf("Name = %q, want %q", got.Name, "renamed")
	}
	if got.APIBase != "https://api.openai.com" {
		t.Errorf("APIBase changed unexpectedly: %q", got.APIBase)
	}
	if got.Token != "tok" {
		t.Errorf("Token changed unexpectedly: %q", got.Token)
	}
}

func TestProviderDelete_RequiresYes_ClearError(t *testing.T) {
	err := (&ProviderDeleteCmdKong{UUID: "some-uuid"}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "-y/--yes") {
		t.Errorf("error = %q, want it to mention -y/--yes", err.Error())
	}
}

func TestProviderDelete_WithYes_Deletes(t *testing.T) {
	am := newTestAppManager(t)
	uuid, err := addProviderForTest(am, "openai", "https://api.openai.com", "tok", protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("addProviderForTest: %v", err)
	}

	withSilencedStdout(t, func() {
		err = (&ProviderDeleteCmdKong{UUID: uuid, Yes: true}).Run(am)
	})
	if err != nil {
		t.Fatalf("ProviderDeleteCmdKong.Run: %v", err)
	}
	if _, getErr := usecase.NewProviderUseCase(am.GetGlobalConfig()).Get(usecase.GetProviderRequest{UUID: uuid}); getErr == nil {
		t.Error("provider still exists after delete")
	}
}
