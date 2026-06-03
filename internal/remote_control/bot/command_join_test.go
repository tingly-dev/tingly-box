package bot

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot"
)

func TestResolveGroupChatID_Feishu(t *testing.T) {
	for _, p := range []imbot.Platform{imbot.PlatformFeishu, imbot.PlatformLark} {
		ctx := &imbot.HandlerContext{Platform: p}

		got, err := resolveGroupChatID(ctx, "  oc_abc123  ")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", p, err)
		}
		if got != "oc_abc123" {
			t.Errorf("%s: expected trimmed chat id, got %q", p, got)
		}

		if _, err := resolveGroupChatID(ctx, "not_a_chat"); err == nil {
			t.Errorf("%s: expected error for non-oc_ id", p)
		}
	}
}

func TestResolveGroupChatID_UnsupportedPlatform(t *testing.T) {
	ctx := &imbot.HandlerContext{Platform: imbot.Platform("discord")}
	_, err := resolveGroupChatID(ctx, "anything")
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected unsupported-platform error, got %v", err)
	}
}
