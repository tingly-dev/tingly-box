package core

import "testing"

// TestPlatformBehaviorPairingDefaults pins which platforms enforce TOFU pairing
// when the operator has expressed no preference. Getting this wrong in the
// permissive direction means anyone holding a leaked bot token gets full DM
// command access, so the table is worth an explicit test.
func TestPlatformBehaviorPairingDefaults(t *testing.T) {
	tokenDM := []Platform{PlatformTelegram, PlatformDiscord, PlatformSlack}
	for _, p := range tokenDM {
		if !GetPlatformBehavior(p).RequiresPairingByDefault {
			t.Errorf("%s hands out DM access to any token holder and must default to pairing", p)
		}
	}

	others := []Platform{PlatformFeishu, PlatformLark, PlatformDingTalk, PlatformWecom, PlatformWeixin, PlatformTingly}
	for _, p := range others {
		if GetPlatformBehavior(p).RequiresPairingByDefault {
			t.Errorf("%s authenticates through the platform itself; pairing should not be forced", p)
		}
	}
}

func TestPlatformBehaviorVerbose(t *testing.T) {
	// Weixin ties each outbound message to a single inbound reply context, so
	// a running commentary of intermediate messages cannot be delivered.
	if !GetPlatformBehavior(PlatformWeixin).SuppressVerbose {
		t.Error("weixin cannot carry intermediate progress messages")
	}
	for _, p := range []Platform{PlatformTelegram, PlatformFeishu, PlatformTingly} {
		if GetPlatformBehavior(p).SuppressVerbose {
			t.Errorf("%s supports intermediate messages", p)
		}
	}
}

// TestPlatformBehaviorUnknownIsConservative covers the property that makes the
// zero value safe: an unknown platform must not accidentally opt into anything.
func TestPlatformBehaviorUnknownIsConservative(t *testing.T) {
	b := GetPlatformBehavior(Platform("some-future-platform"))
	if b.RequiresPairingByDefault || b.SuppressVerbose {
		t.Errorf("unknown platform got non-zero behavior: %+v", b)
	}
}
