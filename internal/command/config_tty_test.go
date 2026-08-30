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

func TestConfigRuleUpdate_NoTTY_ClearErrorEvenWithUUID(t *testing.T) {
	// runRuleUpdateService always re-prompts for the new service, so a UUID
	// argument alone must not bypass the guard.
	withNonTTYStdin(t)

	err := (&ConfigRuleUpdateCmdKong{UUID: "some-uuid"}).Run(nil)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "no TTY") {
		t.Errorf("error = %q, want it to mention the missing TTY", err.Error())
	}
}

func TestConfigRuleDelete_NoTTY_ClearErrorEvenWithUUID(t *testing.T) {
	// runRuleDelete always confirms with [y/N], so a UUID argument alone
	// must not bypass the guard.
	withNonTTYStdin(t)

	err := (&ConfigRuleDeleteCmdKong{UUID: "some-uuid"}).Run(nil)
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
