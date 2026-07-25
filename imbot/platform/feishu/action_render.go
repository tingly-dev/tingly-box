package feishu

import (
	"context"
	"fmt"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
)

// sendActionCard renders text plus a neutral action set as a Feishu
// interactive card.
//
// This replaces the previous path, which type-switched on whatever the caller
// had stuffed into Metadata["replyMarkup"]. Callers in practice supplied a
// go-telegram models.InlineKeyboardMarkup, which matched none of the cases, so
// every remote-control keyboard reached Feishu users as a card with no buttons.
func (b *Bot) sendActionCard(ctx context.Context, target string, opts *core.SendMessageOptions, set *core.ActionSet) (*core.SendResult, error) {
	if b.client == nil {
		return nil, fmt.Errorf("bot client is nil")
	}
	if b.client.Im == nil {
		return nil, fmt.Errorf("client.Im is nil")
	}

	card := buildActionCard(opts.Text, set)
	cardJSON, err := card.String()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize card: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(getReceiveIdType(target)).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(target).
			MsgType("interactive").
			Content(cardJSON).
			Build()).
		Build()

	resp, err := b.client.Im.Message.Create(ctx, req)
	if err != nil {
		return nil, core.WrapError(err, core.Platform(b.domain), core.ErrPlatformError)
	}
	if resp.Code != 0 {
		return nil, core.NewBotError(core.ErrPlatformError, fmt.Sprintf("API error: %s", resp.Msg), false)
	}

	b.UpdateLastActivity()
	return &core.SendResult{MessageID: b.extractMessageIDFromResponse(resp)}, nil
}

// buildActionCard builds a Lark interactive card from text and a neutral
// action set.
func buildActionCard(text string, set *core.ActionSet) *larkcard.MessageCard {
	elements := []larkcard.MessageCardElement{
		larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(text)),
	}

	// Feishu lays actions out itself, so rows are flattened.
	var buttons []larkcard.MessageCardActionElement
	for _, action := range set.Flatten() {
		if btn, ok := buildActionButton(action); ok {
			buttons = append(buttons, btn)
		}
	}

	if len(buttons) > 0 {
		layout := larkcard.MessageCardActionLayoutFlow
		elements = append(elements, larkcard.NewMessageCardAction().
			Layout(&layout).
			Actions(buttons))
	}

	wideScreen := true
	return larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(wideScreen)).
		Elements(elements).
		Build()
}

// buildActionButton renders one neutral action as a Feishu card button.
//
// Feishu has no Tier 3 extensions of its own yet; an action carrying another
// platform's Ext is rendered from its neutral fields when it has them, and
// otherwise honours its FallbackPolicy. FallbackAsText is handled by the
// caller, not here, because it changes the message body rather than the
// buttons.
func buildActionButton(action core.Action) (larkcard.MessageCardActionElement, bool) {
	if action.Label == "" {
		return nil, false
	}

	// A link-only action with nothing to call back stays a link.
	if action.IsLink() {
		if action.Fallback == core.FallbackDrop && action.Kind == core.ActionOpenMiniApp {
			return nil, false
		}
		return larkcard.NewMessageCardEmbedButton().
			Text(larkcard.NewMessageCardPlainText().Content(action.Label)).
			Type(larkcard.MessageCardButtonTypeDefault).
			MultiUrl(larkcard.NewMessageCardURL().Url(action.URL)), true
	}

	if action.CallbackData == "" {
		// Nothing to send back and nowhere to go.
		return nil, false
	}

	return larkcard.NewMessageCardEmbedButton().
		Text(larkcard.NewMessageCardPlainText().Content(action.Label)).
		Type(larkcard.MessageCardButtonTypeDefault).
		Value(map[string]interface{}{
			"callback": action.CallbackData,
			"actionId": action.ID,
		}), true
}

// actionSetFromLegacyMarkup accepts the shapes that used to be pushed through
// Metadata["replyMarkup"] and normalises them to an action set. Kept for one
// release while call sites migrate to SendMessageOptions.Actions.
func actionSetFromLegacyMarkup(raw any) *core.ActionSet {
	switch m := raw.(type) {
	case *core.ActionSet:
		return m
	case interaction.InlineKeyboardMarkup:
		return m.ToActionSet()
	case *interaction.InlineKeyboardMarkup:
		if m == nil {
			return nil
		}
		return m.ToActionSet()
	case map[string]interface{}:
		return actionSetFromMap(m)
	}
	return nil
}

// actionSetFromMap handles a keyboard that arrived as decoded JSON.
func actionSetFromMap(m map[string]interface{}) *core.ActionSet {
	rows, ok := m["inline_keyboard"].([]interface{})
	if !ok {
		return nil
	}
	set := core.NewActionSet()
	for _, row := range rows {
		rowArray, ok := row.([]interface{})
		if !ok {
			continue
		}
		actions := make([]core.Action, 0, len(rowArray))
		for _, btn := range rowArray {
			btnMap, ok := btn.(map[string]interface{})
			if !ok {
				continue
			}
			label, _ := btnMap["text"].(string)
			callback, _ := btnMap["callback_data"].(string)
			url, _ := btnMap["url"].(string)
			actions = append(actions, core.Action{Label: label, CallbackData: callback, URL: url})
		}
		set.AddRow(actions...)
	}
	return set
}
