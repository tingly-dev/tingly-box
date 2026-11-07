package bot_test

import (
	"testing"

	"github.com/tingly-dev/tingly-box/remote/bot"
)

// TestBotSetting_IsRequirePairing verifies the helper returns the right
// answer for the explicit/explicit/platform-default tri-state.
func TestBotSetting_IsRequirePairing(t *testing.T) {
	yes := true
	no := false
	cases := []struct {
		name     string
		v        *bool
		platform string
		want     bool
	}{
		// Explicit values always win, regardless of platform.
		{"explicit true on telegram", &yes, "telegram", true},
		{"explicit true on feishu", &yes, "feishu", true},
		{"explicit false on telegram", &no, "telegram", false},
		{"explicit false on feishu", &no, "feishu", false},

		// Platform defaults: token-DM platforms enforce, others don't.
		{"nil on telegram defaults on", nil, "telegram", true},
		{"nil on discord defaults on", nil, "discord", true},
		{"nil on slack defaults on", nil, "slack", true},
		{"nil on feishu defaults off", nil, "feishu", false},
		{"nil on dingtalk defaults off", nil, "dingtalk", false},
		{"nil on whatsapp defaults off", nil, "whatsapp", false},
		{"nil on weixin defaults off", nil, "weixin", false},
		{"nil on empty platform defaults off", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := bot.BotSetting{RequirePairing: tc.v, Platform: tc.platform}
			if got := b.IsRequirePairing(); got != tc.want {
				t.Fatalf("IsRequirePairing()=%v want %v", got, tc.want)
			}
		})
	}
}
