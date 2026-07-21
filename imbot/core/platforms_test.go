package core

import "testing"

// TestPlatformDescriptorsIntegrity guards the single-source table against
// obvious mistakes: duplicate ids, empty names, and capability entries with a
// zero text limit.
func TestPlatformDescriptorsIntegrity(t *testing.T) {
	seen := make(map[Platform]bool)
	for _, d := range platformDescriptors {
		if d.ID == "" {
			t.Errorf("descriptor with empty ID: %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate descriptor for platform %q", d.ID)
		}
		seen[d.ID] = true

		if d.DisplayName == "" {
			t.Errorf("platform %q has empty DisplayName", d.ID)
		}
		if d.Capabilities != nil && d.Capabilities.TextLimit <= 0 {
			t.Errorf("platform %q has capabilities with non-positive TextLimit", d.ID)
		}
	}

	if len(platformByID) != len(platformDescriptors) {
		t.Errorf("platformByID has %d entries, want %d", len(platformByID), len(platformDescriptors))
	}
}

// TestPlatformNamesDerived verifies PlatformNames stays in lockstep with the
// descriptor table (it is derived, so this catches accidental divergence).
func TestPlatformNamesDerived(t *testing.T) {
	if len(PlatformNames) != len(platformDescriptors) {
		t.Fatalf("PlatformNames has %d entries, want %d", len(PlatformNames), len(platformDescriptors))
	}
	for _, d := range platformDescriptors {
		if got := PlatformNames[d.ID]; got != d.DisplayName {
			t.Errorf("PlatformNames[%q] = %q, want %q", d.ID, got, d.DisplayName)
		}
	}
}

// TestReservedPlatformsValid documents that reserved-but-not-configurable
// platforms remain known platform identifiers (they have capability entries).
func TestReservedPlatformsValid(t *testing.T) {
	for _, p := range []string{"signal", "googlechat", "bluebubbles"} {
		if !IsValidPlatform(p) {
			t.Errorf("IsValidPlatform(%q) = false, want true (reserved platform)", p)
		}
	}
}

func TestGetPlatformCapabilities(t *testing.T) {
	cases := map[Platform]int{
		PlatformTelegram: 4096,
		PlatformDiscord:  2000,
		PlatformSlack:    40000,
		PlatformTingly:   65536,
	}
	for p, wantLimit := range cases {
		if got := GetPlatformCapabilities(p).TextLimit; got != wantLimit {
			t.Errorf("GetPlatformCapabilities(%q).TextLimit = %d, want %d", p, got, wantLimit)
		}
	}

	// Weixin has no explicit entry and must fall back to the default.
	weixin := GetPlatformCapabilities(PlatformWeixin)
	if weixin != defaultPlatformCapabilities {
		t.Errorf("GetPlatformCapabilities(weixin) should return the shared default")
	}
	if got := GetPlatformCapabilities(Platform("nope")); got != defaultPlatformCapabilities {
		t.Errorf("GetPlatformCapabilities(unknown) should return the shared default")
	}

	if !GetPlatformCapabilities(PlatformWecom).SupportsFeature("streaming") {
		t.Errorf("WeCom should advertise the streaming feature")
	}
}

func TestResolveReaction(t *testing.T) {
	cases := []struct {
		platform Platform
		token    ReactionToken
		want     string
	}{
		{PlatformTelegram, ReactionDone, "✅"},
		{PlatformSlack, ReactionDone, "white_check_mark"},
		{PlatformSlack, ReactionReceived, "eyes"},
		{PlatformFeishu, ReactionDone, "DONE"},
		{PlatformLark, ReactionError, "CrossMark"},
		// Platforms without a reaction map fall back to the token string.
		{PlatformWeixin, ReactionDone, "done"},
		{PlatformGoogleChat, ReactionLike, "like"},
	}
	for _, c := range cases {
		if got := ResolveReaction(c.platform, c.token); got != c.want {
			t.Errorf("ResolveReaction(%q, %q) = %q, want %q", c.platform, c.token, got, c.want)
		}
	}
}
