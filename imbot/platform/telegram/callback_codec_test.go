package telegram

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// TestShortPayloadKeepsFlatEncoding pins the wire format for payloads that
// fit. Buttons rendered by earlier releases carry exactly these bytes, so
// changing this would strand every keyboard already sitting in a chat.
func TestShortPayloadKeepsFlatEncoding(t *testing.T) {
	v := newCallbackVault()
	got := v.encodeCallbackData(core.NewPayload("bind", "up"))
	if got != "bind:up" {
		t.Fatalf("encoded = %q, want bind:up", got)
	}
	back, ok := v.decodeCallbackData(got)
	if !ok || back.Name() != "bind" || back.Arg(1) != "up" {
		t.Fatalf("round trip = %v (ok=%v)", back, ok)
	}
}

// TestOversizePayloadStaysWithinTelegramLimit is the defect this seam exists
// to close. A bind confirmation carries an absolute path; the old encoding put
// it straight into callback_data, so any path over 52 bytes pushed the button
// past Telegram's cap and sendMessage failed for the whole message — the user
// saw nothing at all, with no error anywhere that named the cause.
func TestOversizePayloadStaysWithinTelegramLimit(t *testing.T) {
	v := newCallbackVault()
	longPath := "/home/somebody/projects/an-organisation/a-fairly-long-repository-name/nested/deeper"
	payload := core.NewPayload("bind", "create", longPath)

	if len(payload.FlatCallbackData()) <= telegramCallbackDataLimit {
		t.Fatalf("test fixture is not actually oversized (%d bytes)", len(payload.FlatCallbackData()))
	}

	encoded := v.encodeCallbackData(payload)
	if len(encoded) > telegramCallbackDataLimit {
		t.Fatalf("encoded to %d bytes, over Telegram's %d limit: %q",
			len(encoded), telegramCallbackDataLimit, encoded)
	}

	back, ok := v.decodeCallbackData(encoded)
	if !ok {
		t.Fatal("token did not resolve")
	}
	if back.Arg(2) != longPath {
		t.Errorf("path = %q, want %q", back.Arg(2), longPath)
	}
}

// TestPayloadWithSeparatorRoundTrips covers what FormatDirPath used to work
// around by substituting a NUL byte — a substitution that produced bytes no
// JSON string can hold, so it broke the platforms whose button values are
// JSON. Segments carry the colon intact instead.
func TestPayloadWithSeparatorRoundTrips(t *testing.T) {
	v := newCallbackVault()
	weird := "/mnt/c:/Users/someone/project"
	encoded := v.encodeCallbackData(core.NewPayload("bind", "dir", weird))

	back, ok := v.decodeCallbackData(encoded)
	if !ok {
		t.Fatal("did not resolve")
	}
	if back.Arg(2) != weird {
		t.Errorf("path = %q, want %q", back.Arg(2), weird)
	}
	if strings.Contains(encoded, "\x00") {
		t.Error("encoding must not emit NUL bytes")
	}
}

// TestReservedPrefixIsNotConfusedForAToken guards the one ambiguity the token
// scheme introduces: an application payload whose first segment starts with
// the reserved marker must not be handed back as a token lookup.
func TestReservedPrefixIsNotConfusedForAToken(t *testing.T) {
	v := newCallbackVault()
	payload := core.NewPayload(tokenPrefix+"mention", "reply")

	encoded := v.encodeCallbackData(payload)
	back, ok := v.decodeCallbackData(encoded)
	if !ok {
		t.Fatal("did not resolve")
	}
	if back.Name() != tokenPrefix+"mention" || back.Arg(1) != "reply" {
		t.Errorf("round trip lost the payload: %v", back)
	}
}

// TestUnresolvableTokenIsReportedNotSilent: an evicted or post-restart token
// must be distinguishable from valid data, so the bot can say the button is
// dead rather than absorb the tap.
func TestUnresolvableTokenIsReportedNotSilent(t *testing.T) {
	v := newCallbackVault()
	if _, ok := v.decodeCallbackData(tokenPrefix + "zzzz"); ok {
		t.Error("an unknown token must not resolve")
	}
	// Plain data from before this encoding existed still decodes.
	if _, ok := v.decodeCallbackData("perm:deny:req-1"); !ok {
		t.Error("legacy flat callback data must still decode")
	}
}

func TestVaultEvictsOldestBeyondCapacity(t *testing.T) {
	v := newCallbackVault()
	v.capacity = 3

	tokens := make([]string, 0, 4)
	for _, name := range []string{"a", "b", "c", "d"} {
		tokens = append(tokens, v.park(core.NewPayload(name)))
	}

	if _, ok := v.resolve(tokens[0]); ok {
		t.Error("the oldest entry should have been evicted")
	}
	for _, tok := range tokens[1:] {
		if _, ok := v.resolve(tok); !ok {
			t.Errorf("token %q should still resolve", tok)
		}
	}
}

// TestBuildInlineKeyboardEncodesPayload checks the renderer goes through the
// codec rather than reading a pre-joined string off the action.
func TestBuildInlineKeyboardEncodesPayload(t *testing.T) {
	b := testBot()
	kb := b.BuildInlineKeyboard(core.NewActionSet().AddRow(
		core.Action{Label: "Up", Payload: core.NewPayload("bind", "up")},
	))
	if got := kb.InlineKeyboard[0][0].CallbackData; got != "bind:up" {
		t.Errorf("callback data = %q, want bind:up", got)
	}
}
