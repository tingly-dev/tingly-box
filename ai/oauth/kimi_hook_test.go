package oauth

import (
	"context"
	"net/http"
	"testing"
)

func TestKimiHook_BeforeAuth_NoOp(t *testing.T) {
	hook := &KimiHook{}
	params := map[string]string{"existing": "value"}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if len(params) != 1 || params["existing"] != "value" {
		t.Errorf("expected BeforeAuth to be a no-op, got %v", params)
	}
}

func TestKimiHook_BeforeToken_SetsDeviceHeaders(t *testing.T) {
	hook := &KimiHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}

	if got := header.Get("X-Msh-Platform"); got != "kimi_cli" {
		t.Errorf("expected X-Msh-Platform=kimi_cli, got %q", got)
	}
	if got := header.Get("X-Msh-Version"); got != "1.10.6" {
		t.Errorf("expected X-Msh-Version=1.10.6, got %q", got)
	}
	if header.Get("X-Msh-Device-Name") == "" {
		t.Error("expected non-empty X-Msh-Device-Name")
	}
	if header.Get("X-Msh-Device-Model") == "" {
		t.Error("expected non-empty X-Msh-Device-Model")
	}
	if header.Get("X-Msh-Os-Version") == "" {
		t.Error("expected non-empty X-Msh-Os-Version")
	}
}

func TestKimiHook_AfterToken_NoOp(t *testing.T) {
	hook := &KimiHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", http.DefaultClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata, got %v", meta)
	}
}

func TestKimiDeviceModel_MatchesRuntimeGOOS(t *testing.T) {
	got := KimiDeviceModel()
	if got == "" {
		t.Fatal("expected non-empty device model")
	}
}

func TestKimiOsVersion_ReturnsKnownValue(t *testing.T) {
	got := KimiOsVersion()
	if got == "" {
		t.Fatal("expected non-empty OS version")
	}
}
