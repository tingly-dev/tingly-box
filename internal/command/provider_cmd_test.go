package command

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

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
