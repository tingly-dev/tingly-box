package feishu

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// A press on a card must come from the chat the card was sent to, under the
// same id the chat's messages carry. A one-to-one chat's messages carry the
// user's id (the reply target), while Feishu names the chat of a card press
// by its open_chat_id, so a press read as coming from the open_chat_id is a
// press from a chat nothing knows: Projects answered "this chat is not bound
// to that thread" to every button of a DM bound to a project.

// cardButtonValue renders a card sent to target with one button and returns
// that button's value as Feishu hands it back, after a trip through JSON.
func cardButtonValue(t *testing.T, target string, payload core.Payload) map[string]interface{} {
	t.Helper()
	set := core.NewActionSet()
	set.AddRow(core.Action{Label: "Allow", Payload: payload})
	raw, err := buildActionCard("Allow Bash?", set, target).String()
	require.NoError(t, err)
	var card struct {
		Elements []struct {
			Actions []struct {
				Value map[string]interface{} `json:"value"`
			} `json:"actions"`
		} `json:"elements"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &card))
	for _, el := range card.Elements {
		for _, a := range el.Actions {
			return a.Value
		}
	}
	t.Fatal("the card has no button")
	return nil
}

func pressCard(t *testing.T, b *Bot, value map[string]interface{}, openChatID, userID, openID string) core.Message {
	t.Helper()
	received := make(chan core.Message, 1)
	b.OnMessage(func(m core.Message) { received <- m })
	op := &callback.Operator{OpenID: openID}
	if userID != "" {
		op.UserID = &userID
	}
	_, err := b.handleCardActionTrigger(context.Background(), &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Operator: op,
			Action:   &callback.CallBackAction{Value: value},
			Context:  &callback.Context{OpenChatID: openChatID, OpenMessageID: "om_card"},
		},
	})
	require.NoError(t, err)
	select {
	case m := <-received:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("the press was not emitted")
		return core.Message{}
	}
}

func newTestBot() *Bot {
	return &Bot{BaseBot: core.NewBaseBot(&core.Config{Platform: core.PlatformFeishu}), domain: DomainFeishu}
}

func directMessage(b *Bot, chatID, userID string) *core.Message {
	str := func(s string) *string { return &s }
	return b.convertLarkMessageToCore(&larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderId: &larkim.UserId{UserId: str(userID), OpenId: str("ou_" + userID)}},
		Message: &larkim.EventMessage{
			MessageId:   str("om_text"),
			ChatId:      str(chatID),
			ChatType:    str("p2p"),
			MessageType: str("text"),
			Content:     str(`{"text":"hi"}`),
		},
	}})
}

func TestADirectChatsCardPressComesFromThatChat(t *testing.T) {
	b := newTestBot()
	typed := directMessage(b, "oc_p2p", "u42")
	require.Equal(t, "u42", typed.Recipient.ID, "a DM's messages carry the user's id")

	// Projects sends its card to the id the chat's messages carry.
	value := cardButtonValue(t, typed.Recipient.ID, core.NewPayload("pj", "thread-1", "req-1", "allow"))
	pressed := pressCard(t, b, value, "oc_p2p", "u42", "ou_u42")

	assert.Equal(t, typed.Recipient.ID, pressed.Recipient.ID)
	assert.Equal(t, core.Payload{"pj", "thread-1", "req-1", "allow"}, pressed.Payload)
}

// Without the user_id scope a DM's messages carry the open_id; a press does too.
func TestADirectChatsCardPressComesFromThatChatByOpenID(t *testing.T) {
	b := newTestBot()
	value := cardButtonValue(t, "ou_u42", core.NewPayload("pj", "t", "r", "deny"))
	assert.Equal(t, "ou_u42", pressCard(t, b, value, "oc_p2p", "", "ou_u42").Recipient.ID)
}

func TestAGroupsCardPressComesFromTheGroup(t *testing.T) {
	b := newTestBot()
	value := cardButtonValue(t, "oc_group", core.NewPayload("pj", "t", "r", "allow"))
	assert.Equal(t, "oc_group", pressCard(t, b, value, "oc_group", "u42", "ou_u42").Recipient.ID)
}

// A card forwarded elsewhere does not speak for the chat it was sent to: the
// press is from the chat it was pressed in.
func TestAForwardedCardSpeaksForTheChatItIsPressedIn(t *testing.T) {
	b := newTestBot()
	dm := cardButtonValue(t, "u42", core.NewPayload("pj", "t", "r", "allow"))
	assert.Equal(t, "oc_other_p2p", pressCard(t, b, dm, "oc_other_p2p", "u7", "ou_u7").Recipient.ID, "another user pressing a DM card")
	group := cardButtonValue(t, "oc_group", core.NewPayload("pj", "t", "r", "allow"))
	assert.Equal(t, "oc_elsewhere", pressCard(t, b, group, "oc_elsewhere", "u42", "ou_u42").Recipient.ID, "a group card pressed in another chat")
}

// A card rendered before cards named their chat still answers from the
// open_chat_id, as before.
func TestACardWithoutItsChatComesFromTheOpenChat(t *testing.T) {
	b := newTestBot()
	value := map[string]interface{}{payloadValueKey: []interface{}{"pj", "t", "r", "allow"}}
	assert.Equal(t, "oc_p2p", pressCard(t, b, value, "oc_p2p", "u42", "ou_u42").Recipient.ID)
}
