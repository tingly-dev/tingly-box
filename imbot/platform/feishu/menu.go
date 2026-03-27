package feishu

import (
	"context"
	"fmt"

	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	"github.com/tingly-dev/tingly-box/imbot/interaction"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/menu"
)

// MenuAdapter implements menu support for Feishu platform
type MenuAdapter struct {
	domain Domain // "feishu" or "lark"
}

// NewMenuAdapter creates a new Feishu menu adapter
func NewMenuAdapter() *MenuAdapter {
	return &MenuAdapter{
		domain: DomainFeishu,
	}
}

// NewMenuAdapterWithDomain creates a menu adapter with a specific domain
func NewMenuAdapterWithDomain(domain Domain) *MenuAdapter {
	return &MenuAdapter{
		domain: domain,
	}
}

// Supports checks if this adapter supports Feishu or Lark
func (a *MenuAdapter) Supports(platform core.Platform) bool {
	return platform == core.PlatformFeishu || platform == core.PlatformLark
}

// ConvertMenu converts a generic Menu to Feishu card format
func (a *MenuAdapter) ConvertMenu(m *menu.Menu) (interface{}, error) {
	// Validate menu
	if m == nil {
		return nil, menu.ErrInvalidContext
	}

	// Use menu helper methods
	if m.IsQuickActions() {
		return a.convertToQuickActions(m)
	} else {
		// Default to interactive card (for IsInlineKeyboard, IsAuto, or any other type)
		return a.convertToInteractiveCard(m)
	}
}

// convertToInteractiveCard converts menu to Feishu interactive card
func (a *MenuAdapter) convertToInteractiveCard(m *menu.Menu) (interface{}, error) {
	// Build card elements
	var elements []larkcard.MessageCardElement

	// Add text content
	if m.Title != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(m.Title))
		elements = append(elements, divElement)
	}

	// Build action buttons
	var buttons []larkcard.MessageCardActionElement
	for _, row := range m.Items {
		for _, item := range row {
			button := a.buildCardButton(m.ID, item)
			buttons = append(buttons, button)
		}
	}

	// Add action element if there are buttons
	if len(buttons) > 0 {
		layout := larkcard.MessageCardActionLayoutFlow
		action := larkcard.NewMessageCardAction().
			Layout(&layout).
			Actions(buttons)
		elements = append(elements, action)
	}

	// Build the card
	wideScreen := true
	card := larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(wideScreen)).
		Elements(elements)

	// Return as JSON string in map for compatibility
	cardStr, err := card.String()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize card: %w", err)
	}

	return map[string]interface{}{
		"card_json": cardStr,
		"type":      "interactive_card",
	}, nil
}

// convertToQuickActions converts menu to Feishu quick actions format
func (a *MenuAdapter) convertToQuickActions(m *menu.Menu) (interface{}, error) {
	// Quick actions are typically defined in the app configuration
	type QuickAction struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Value string `json:"value"`
	}

	actions := make([]QuickAction, 0)
	for _, row := range m.Items {
		for _, item := range row {
			actions = append(actions, QuickAction{
				ID:    item.ID,
				Label: item.Label,
				Value: item.Value,
			})
		}
	}

	return map[string]interface{}{
		"type":    "quick_actions",
		"actions": actions,
	}, nil
}

// buildCardButton creates a Feishu card button from a menu item
func (a *MenuAdapter) buildCardButton(menuID string, item menu.MenuItem) larkcard.MessageCardActionElement {
	button := larkcard.NewMessageCardEmbedButton().
		Text(larkcard.NewMessageCardPlainText().Content(item.Label))

	// Add icon if provided
	if item.Icon != "" {
		button.Text(larkcard.NewMessageCardPlainText().Content(item.Icon + " " + item.Label))
	}

	// Set button type based on item state
	if item.Disabled {
		button.Type(larkcard.MessageCardButtonTypeDefault)
	} else {
		button.Type(larkcard.MessageCardButtonTypePrimary)
	}

	// Set value/callback data
	callbackData := interaction.FormatCallbackData(menuID, item.ID, item.Value)
	button.Value(map[string]interface{}{
		"action": callbackData,
		"menuId": menuID,
		"itemId": item.ID,
		"value":  item.Value,
	})

	return button
}

// ShowMenu displays a menu on Feishu
func (a *MenuAdapter) ShowMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, m *menu.Menu) (*menu.MenuResult, error) {
	// Convert menu to Feishu format
	cardData, err := a.ConvertMenu(m)
	if err != nil {
		return menu.NewErrorMenuResult(err), nil
	}

	// Prepare send options with card
	opts := &core.SendMessageOptions{
		Text: m.Title,
		Metadata: map[string]interface{}{
			"menuId":      m.ID,
			"replyMarkup": cardData,
			"cardType":    "interactive",
		},
	}

	// For Feishu, we need to handle card format specially
	if cardMap, ok := cardData.(map[string]interface{}); ok {
		if cardJSON, ok := cardMap["card_json"].(string); ok {
			opts.ParseMode = core.ParseModeMarkdown
			opts.Metadata["card_json"] = cardJSON
		}
	}

	// Send the message
	result, err := bot.SendMessage(ctx, menuCtx.ChatID, opts)
	if err != nil {
		return menu.NewErrorMenuResult(err), nil
	}

	return &menu.MenuResult{
		Success:   true,
		MessageID: result.MessageID,
		MenuID:    m.ID,
		RawData: map[string]interface{}{
			"card": cardData,
		},
	}, nil
}

// HideMenu removes a menu from Feishu chat
func (a *MenuAdapter) HideMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, menuID string) error {
	if menuCtx.MessageID != "" {
		return bot.DeleteMessage(ctx, menuCtx.MessageID)
	}

	_, err := bot.SendText(ctx, menuCtx.ChatID, "Menu closed")
	return err
}

// UpdateMenu updates an existing menu on Feishu
func (a *MenuAdapter) UpdateMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, m *menu.Menu) error {
	if menuCtx.MessageID != "" {
		return bot.EditMessage(ctx, menuCtx.MessageID, m.Title)
	}

	_, err := a.ShowMenu(ctx, bot, menuCtx, m)
	return err
}

// ParseAction parses a Feishu card callback into a MenuAction
func (a *MenuAdapter) ParseAction(msg *core.Message) (*menu.MenuAction, error) {
	// Check if message has action callback data in metadata
	if action, ok := msg.Metadata["action"].(string); ok {
		parts := interaction.ParseCallbackData(action)
		if len(parts) >= 3 {
			return &menu.MenuAction{
				MenuID:    parts[0],
				ItemID:    parts[1],
				Value:     parts[2],
				UserID:    msg.Sender.ID,
				ChatID:    msg.Recipient.ID,
				MessageID: msg.ID,
				Action:    "callback",
			}, nil
		}
	}

	// Check for card interaction in metadata
	if cardAction, ok := msg.Metadata["card_action"].(map[string]interface{}); ok {
		if action, ok := cardAction["action"].(string); ok {
			parts := interaction.ParseCallbackData(action)
			if len(parts) >= 3 {
				return &menu.MenuAction{
					MenuID:    parts[0],
					ItemID:    parts[1],
					Value:     parts[2],
					UserID:    msg.Sender.ID,
					ChatID:    msg.Recipient.ID,
					MessageID: msg.ID,
					Action:    "card_callback",
				}, nil
			}
		}
	}

	// Check for text-based menu selection
	if textContent, ok := msg.Content.(*core.TextContent); ok {
		return &menu.MenuAction{
			Value:     textContent.Text,
			UserID:    msg.Sender.ID,
			ChatID:    msg.Recipient.ID,
			MessageID: msg.ID,
			Action:    "text_selection",
		}, nil
	}

	return nil, menu.ErrNotMenuAction
}

// SendInteractiveCard sends an interactive card menu to a chat
func (a *MenuAdapter) SendInteractiveCard(ctx context.Context, bot core.Bot, chatID, text string, m *menu.Menu) (*menu.MenuResult, error) {
	m.SetInlineKeyboard()

	m.Title = text
	menuCtx := menu.NewMenuContext(chatID, core.PlatformFeishu)

	return a.ShowMenu(ctx, bot, menuCtx, m)
}

// CreateQuickActionConfig creates a quick action configuration for Feishu
func (a *MenuAdapter) CreateQuickActionConfig(m *menu.Menu) (map[string]interface{}, error) {
	if !m.IsQuickActions() {
		m.SetQuickActions()
	}

	result, err := a.ConvertMenu(m)
	if err != nil {
		return nil, err
	}

	if resultMap, ok := result.(map[string]interface{}); ok {
		return resultMap, nil
	}

	return nil, fmt.Errorf("failed to convert menu to map")
}

// BuildButtonElement creates a button element for Feishu cards
func (a *MenuAdapter) BuildButtonElement(menuID string, item menu.MenuItem) larkcard.MessageCardActionElement {
	return a.buildCardButton(menuID, item)
}

// BuildCardFromMenu builds a complete Feishu card from a menu
func (a *MenuAdapter) BuildCardFromMenu(m *menu.Menu) (*larkcard.MessageCard, error) {
	var elements []larkcard.MessageCardElement

	// Add text content
	if m.Title != "" {
		divElement := larkcard.NewMessageCardDiv().
			Text(larkcard.NewMessageCardLarkMd().Content(m.Title))
		elements = append(elements, divElement)
	}

	// Build action buttons
	var buttons []larkcard.MessageCardActionElement
	for _, row := range m.Items {
		for _, item := range row {
			button := a.buildCardButton(m.ID, item)
			buttons = append(buttons, button)
		}
	}

	// Add action element if there are buttons
	if len(buttons) > 0 {
		layout := larkcard.MessageCardActionLayoutFlow
		action := larkcard.NewMessageCardAction().
			Layout(&layout).
			Actions(buttons)
		elements = append(elements, action)
	}

	// Build the card
	wideScreen := true
	return larkcard.NewMessageCard().
		Config(larkcard.NewMessageCardConfig().WideScreenMode(wideScreen)).
		Elements(elements).
		Build(), nil
}

// GetCardJSON returns the JSON string representation of a card
func (a *MenuAdapter) GetCardJSON(m *menu.Menu) (string, error) {
	card, err := a.BuildCardFromMenu(m)
	if err != nil {
		return "", err
	}

	return card.String()
}
