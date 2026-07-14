package client

import (
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

func TestApplyCodexDefaultsToParams_FastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol:fast"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to be stripped to %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != responses.ResponseNewParamsServiceTierPriority {
		t.Fatalf("expected service tier %q, got %q", responses.ResponseNewParamsServiceTierPriority, req.ServiceTier)
	}
}

func TestApplyCodexDefaultsToParams_NoFastSuffix(t *testing.T) {
	req := responses.ResponseNewParams{Model: "gpt-5.6-sol"}

	applyCodexDefaultsToParams(&req)

	if req.Model != "gpt-5.6-sol" {
		t.Fatalf("expected model to remain %q, got %q", "gpt-5.6-sol", req.Model)
	}
	if req.ServiceTier != "" {
		t.Fatalf("expected empty service tier, got %q", req.ServiceTier)
	}
}
