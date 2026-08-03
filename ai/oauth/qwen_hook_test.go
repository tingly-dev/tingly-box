package oauth

import (
	"context"
	"net/http"
	"testing"
)

func TestQwenHook_BeforeAuth_NoOp(t *testing.T) {
	hook := &QwenHook{}
	params := map[string]string{"existing": "value"}
	if err := hook.BeforeAuth(params); err != nil {
		t.Fatalf("BeforeAuth returned error: %v", err)
	}
	if len(params) != 1 || params["existing"] != "value" {
		t.Errorf("expected BeforeAuth to be a no-op, got %v", params)
	}
}

func TestQwenHook_BeforeToken_SetsRequestID(t *testing.T) {
	hook := &QwenHook{}
	header := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}
	requestID := header.Get("x-request-id")
	if requestID == "" {
		t.Fatal("expected x-request-id to be set")
	}

	// A second call should produce a different UUID (each token request gets
	// its own request id).
	header2 := http.Header{}
	if err := hook.BeforeToken(map[string]string{}, header2); err != nil {
		t.Fatalf("BeforeToken returned error: %v", err)
	}
	if header2.Get("x-request-id") == requestID {
		t.Error("expected distinct x-request-id across calls")
	}
}

func TestQwenHook_AfterToken_NoOp(t *testing.T) {
	hook := &QwenHook{}
	meta, err := hook.AfterToken(context.Background(), "test-token", http.DefaultClient)
	if err != nil {
		t.Fatalf("AfterToken returned error: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil metadata, got %v", meta)
	}
}
