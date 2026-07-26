package feishu

import (
	"context"
	"testing"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// buttonValue renders one action and digs out the value object Feishu will
// hand back when the button is tapped.
func buttonValue(t *testing.T, action core.Action) map[string]interface{} {
	t.Helper()
	el, ok := buildActionButton(action)
	if !ok {
		t.Fatal("action was not rendered as a button")
	}
	btn, ok := el.(*larkcard.MessageCardEmbedButton)
	if !ok {
		t.Fatalf("unexpected button type %T", el)
	}
	return btn.Value_
}

// TestButtonValueCarriesSegments: a Feishu button value is arbitrary JSON, so
// the payload goes in as segments and nothing needs truncating or escaping.
// This is the half of the seam Telegram cannot have.
func TestButtonValueCarriesSegments(t *testing.T) {
	longPath := "/home/somebody/projects/an-organisation/a-long-repository-name/nested"
	value := buttonValue(t, core.Action{
		Label:   "Create",
		Payload: core.NewPayload("bind", "create", longPath),
	})

	got := payloadFromButtonValue(value)
	if got.Name() != "bind" || got.Arg(1) != "create" || got.Arg(2) != longPath {
		t.Fatalf("round trip = %v", got)
	}
}

// TestButtonValueRoundTripsSeparator: the segment survives a colon, which the
// old NUL escape could not express inside JSON at all.
func TestButtonValueRoundTripsSeparator(t *testing.T) {
	weird := "/mnt/c:/Users/someone"
	value := buttonValue(t, core.Action{
		Label:   "Go",
		Payload: core.NewPayload("bind", "dir", weird),
	})
	if got := payloadFromButtonValue(value).Arg(2); got != weird {
		t.Errorf("path = %q, want %q", got, weird)
	}
}

// TestButtonValueAcceptsLegacyFlatCallback keeps buttons rendered by an
// earlier release working after an upgrade: their value objects carry only the
// joined string.
func TestButtonValueAcceptsLegacyFlatCallback(t *testing.T) {
	got := payloadFromButtonValue(map[string]interface{}{"callback": "perm:deny:req-1"})
	if got.Name() != "perm" || got.Arg(2) != "req-1" {
		t.Fatalf("legacy value did not decode: %v", got)
	}
}

// TestButtonValueDecodesJSONRoundTrip covers the shape the SDK actually
// delivers: the value has been through JSON, so the segment array arrives as
// []interface{} rather than []string.
func TestButtonValueDecodesJSONRoundTrip(t *testing.T) {
	got := payloadFromButtonValue(map[string]interface{}{
		payloadValueKey: []interface{}{"bind", "dir", "/tmp/x"},
	})
	if got.Arg(2) != "/tmp/x" {
		t.Fatalf("decoded = %v", got)
	}
}

// TestGetReceiveIdType covers the mapping that decides how a reply is
// addressed. The previous version compared a four-character slice against
// three-character prefixes, so the chat and user cases were unreachable and
// every target was sent as an open_id — including the chat IDs a card
// callback replies to.
func TestGetReceiveIdType(t *testing.T) {
	cases := map[string]string{
		"oc_a1b2c3":         "chat_id",
		"ou_a1b2c3":         "open_id",
		"on_a1b2c3":         "union_id",
		"someone@corp.test": "email",
		"7f3d9a2b":          "user_id",
	}
	for target, want := range cases {
		if got := getReceiveIdType(target); got != want {
			t.Errorf("getReceiveIdType(%q) = %q, want %q", target, got, want)
		}
	}
}

// TestCardActionResponseIsNeverATypedNil guards a trap in how the SDK returns
// this handler's result. It passes the value out through an interface{} and
// tests it against nil; a nil *CardActionTriggerResponse still reads as
// non-nil there and gets marshalled, so Feishu would receive a literal "null"
// body rather than no body. Every return path must carry a real value.
func TestCardActionResponseIsNeverATypedNil(t *testing.T) {
	b := &Bot{BaseBot: core.NewBaseBot(&core.Config{Platform: core.PlatformFeishu}), domain: DomainFeishu}

	for name, event := range map[string]*callback.CardActionTriggerEvent{
		"nil event":     nil,
		"nil inner":     {},
		"empty payload": {Event: &callback.CardActionTriggerRequest{Action: &callback.CallBackAction{Value: map[string]interface{}{}}}},
		"no chat": {Event: &callback.CardActionTriggerRequest{
			Action: &callback.CallBackAction{Value: map[string]interface{}{"callback": "bind:up"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := b.handleCardActionTrigger(context.Background(), event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("returned a nil pointer; the SDK marshals that as \"null\"")
			}
			// Mirror the SDK: the value crosses an interface{} boundary.
			var asAny interface{} = resp
			if asAny == nil {
				t.Fatal("unreachable, but documents what the SDK checks")
			}
		})
	}
}

// TestCardActionWithoutChatIsDropped: the message builder substitutes
// "unknown" for a missing recipient, so emitting would address a chat that
// does not exist rather than failing visibly.
func TestCardActionWithoutChatIsDropped(t *testing.T) {
	b := &Bot{BaseBot: core.NewBaseBot(&core.Config{Platform: core.PlatformFeishu}), domain: DomainFeishu}

	received := make(chan core.Message, 1)
	b.OnMessage(func(m core.Message) {
		received <- m
	})

	_, err := b.handleCardActionTrigger(context.Background(), &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Action: &callback.CallBackAction{Value: map[string]interface{}{"callback": "bind:up"}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case m := <-received:
		t.Fatalf("a chat-less card action was emitted: recipient=%q", m.Recipient.ID)
	default:
	}
}
