package runtime

import (
	"context"
	"testing"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

func TestAdvisorContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	uses := 2
	ac := &coretool.AdvisorContext{
		Messages:      []map[string]any{{"role": "user", "content": "hello"}},
		UsesRemaining: &uses,
	}
	ctx = coretool.WithAdvisorContext(ctx, ac)
	got, ok := coretool.GetAdvisorContext(ctx)
	if !ok {
		t.Fatal("expected advisor context to be present")
	}
	if *got.UsesRemaining != 2 {
		t.Fatalf("expected UsesRemaining=2, got %d", *got.UsesRemaining)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got.Messages))
	}
}
