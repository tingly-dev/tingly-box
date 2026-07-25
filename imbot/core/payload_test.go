package core

import "testing"

// TestPayloadAccessorsAreBoundsSafe: a stale button from an older release may
// carry fewer segments than the handler expects. Reading past the end must
// look like missing data, not panic the receiving handler.
func TestPayloadAccessorsAreBoundsSafe(t *testing.T) {
	var empty Payload
	if empty.Name() != "" || empty.Arg(3) != "" || !empty.IsEmpty() {
		t.Error("empty payload should read as blanks")
	}

	p := NewPayload("bind", "dir")
	if p.Name() != "bind" || p.Arg(1) != "dir" {
		t.Errorf("unexpected segments: %v", p)
	}
	if p.Arg(2) != "" || p.Arg(-1) != "" {
		t.Error("out-of-range reads should be empty")
	}
}

func TestHasSeparator(t *testing.T) {
	if NewPayload("bind", "dir", "/tmp/x").HasSeparator() {
		t.Error("no segment contains a colon")
	}
	if !NewPayload("bind", "dir", "/mnt/c:/x").HasSeparator() {
		t.Error("the colon should be detected")
	}
}

// TestEffectivePayloadPrefersSegments checks the compatibility rule: producers
// that still set only CallbackData keep working, and a producer that sets both
// is read from the segments.
func TestEffectivePayloadPrefersSegments(t *testing.T) {
	legacy := Action{CallbackData: "perm:deny:req-1"}
	if got := legacy.EffectivePayload(); got.Name() != "perm" || got.Arg(2) != "req-1" {
		t.Errorf("legacy action decoded to %v", got)
	}

	both := Action{Payload: NewPayload("a", "b"), CallbackData: "x:y"}
	if got := both.EffectivePayload(); got.Name() != "a" {
		t.Errorf("segments should win, got %v", got)
	}

	if !(Action{}).EffectivePayload().IsEmpty() {
		t.Error("an action with neither should read as empty")
	}
}

// TestIsLinkConsidersPayload: a URL button with no payload is a link, but one
// carrying a payload is a control that happens to have a URL. Getting this
// wrong renders the button as a plain link and the callback never fires.
func TestIsLinkConsidersPayload(t *testing.T) {
	if !(Action{URL: "https://example.test"}).IsLink() {
		t.Error("a bare URL action is a link")
	}
	if (Action{URL: "https://example.test", Payload: NewPayload("go")}).IsLink() {
		t.Error("an action carrying a payload is not a plain link")
	}
	if !(Action{URL: "https://example.test", Payload: NewPayload("go"), Kind: ActionOpenURL}).IsLink() {
		t.Error("an explicit open_url action is a link regardless")
	}
}
