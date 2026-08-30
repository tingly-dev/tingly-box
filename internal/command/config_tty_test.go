package command

import (
	"strings"
	"testing"
)

// These guard the legacy bufio interactive fallbacks documented as
// transitional in .design/tui.md §12 ("not actively developed" but still
// wired to the flag-mode subcommands' no-flag/no-arg paths). Before these
// checks, every one of them would hang or crash with a bare
// "failed to read input: EOF" without a TTY — the same bug class fixed
// earlier for agent show/apply/restore and token view. All of these calls
// return before touching appManager, so nil is fine here.

func TestConfigProviderDelete_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigProviderDeleteCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigProviderUpdate_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigProviderUpdateCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigProviderGet_NoTTYWithoutUUID_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigProviderGetCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
	if !strings.Contains(err.Error(), "config provider get <uuid>") {
		t.Errorf("error = %q, want a usage example", err.Error())
	}
}

func TestConfigProviderGet_NoTTYWithUUID_StillWorks(t *testing.T) {
	// A UUID makes this fully non-interactive; the TTY guard must not fire.
	withNonTTYStdin(t)
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&ConfigProviderGetCmdKong{UUID: "00000000-0000-0000-0000-000000000000"}).Run(am)
	})
	// Expected to fail because the UUID doesn't exist, not because of TTY.
	if err == nil {
		t.Fatal("expected a not-found error, got nil")
	}
	if strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, should not be a TTY error when UUID is given", err.Error())
	}
}

func TestConfigRuleAdd_NoTTYNoFlags_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigRuleAddCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigRuleUpdate_NoTTY_ClearError(t *testing.T) {
	// Both now always open the TUI's Rule mode (no flag form exists for the
	// new service), so there's nothing left to gate on except the TTY check.
	withNonTTYStdin(t)

	err := (&ConfigRuleUpdateCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigRuleDelete_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigRuleDeleteCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigRuleExport_NoTTYWithoutUUID_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigRuleExportCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigRuleExport_NoTTYWithUUID_StillWorks(t *testing.T) {
	// A UUID makes this fully non-interactive; the TTY guard must not fire.
	withNonTTYStdin(t)
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&ConfigRuleExportCmdKong{UUID: "00000000-0000-0000-0000-000000000000"}).Run(am)
	})
	if err == nil {
		t.Fatal("expected a not-found error, got nil")
	}
	if strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, should not be a TTY error when UUID is given", err.Error())
	}
}

func TestConfigProviderAdd_PartialArgs_ClearError(t *testing.T) {
	// Same all-or-nothing shape as config rule add now: 1-3 positional args
	// is always a clear error (never silently mixed with prompts for the
	// rest), regardless of whether a TTY is attached.
	cases := []ConfigProviderAddCmdKong{
		{Name: "openai"},
		{Name: "openai", BaseURL: "https://api.openai.com"},
		{Name: "openai", BaseURL: "https://api.openai.com", Token: "tok"},
	}
	for _, cmd := range cases {
		err := cmd.Run(nil)
		if err == nil {
			t.Fatalf("%+v: expected an error, got nil", cmd)
		}
		if !strings.Contains(err.Error(), "partial arguments") {
			t.Errorf("%+v: error = %q, want it to mention partial arguments", cmd, err.Error())
		}
	}
}

func TestConfigProviderAdd_NoArgsNoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&ConfigProviderAddCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigProviderAdd_AllFourArgs_StillWorksWithoutTTY(t *testing.T) {
	// All four args is the CI path — must not require a TTY.
	withNonTTYStdin(t)
	am := newTestAppManager(t)

	var err error
	withSilencedStdout(t, func() {
		err = (&ConfigProviderAddCmdKong{
			Name: "openai", BaseURL: "https://api.openai.com", Token: "tok", APIStyle: "openai",
		}).Run(am)
	})
	if err != nil {
		t.Errorf("expected success with all four args given, got: %v", err)
	}
}

func TestRemoteAdd_NoTTY_ClearError(t *testing.T) {
	withNonTTYStdin(t)

	err := (&RemoteAddCmdKong{}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}
