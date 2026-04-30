package bot

import (
	"context"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// RequestInteraction sends an interaction request using the new interaction system
// This is a convenience method for BotHandler to request platform-agnostic interactions
func (h *BotHandler) RequestInteraction(ctx context.Context, hCtx HandlerContext, req imbot.InteractionRequest) (*imbot.InteractionResponse, error) {
	// Set the bot and platform info from the handler context
	req.BotUUID = hCtx.BotUUID
	req.Platform = hCtx.Platform
	req.ChatID = hCtx.ChatID

	// Set default timeout if not specified
	if req.Timeout == 0 {
		req.Timeout = 5 * time.Minute
	}

	return h.interaction.RequestInteraction(ctx, req)
}

// RequestConfirmation requests a yes/no confirmation from the user
// Uses the new interaction system with platform-agnostic UI
func (h *BotHandler) RequestConfirmation(ctx context.Context, hCtx HandlerContext, message, requestID string) (bool, error) {
	builder := imbot.NewInteractionBuilder()
	builder.AddConfirm(requestID)

	req := imbot.InteractionRequest{
		ID:           requestID,
		Message:      message,
		ParseMode:    imbot.ParseModeMarkdown,
		Mode:         imbot.ModeAuto,
		Interactions: builder.Build(),
		Timeout:      5 * time.Minute,
	}

	resp, err := h.RequestInteraction(ctx, hCtx, req)
	if err != nil {
		return false, err
	}

	return resp.IsConfirm(), nil
}

// RequestOptionSelection requests the user to select from a list of options
// Uses the new interaction system with platform-agnostic UI
func (h *BotHandler) RequestOptionSelection(ctx context.Context, hCtx HandlerContext, message, requestID string, options []imbot.Option) (int, *imbot.Interaction, error) {
	builder := imbot.NewInteractionBuilder()
	builder.AddOptions(requestID, options)

	req := imbot.InteractionRequest{
		ID:           requestID,
		Message:      message,
		ParseMode:    imbot.ParseModeMarkdown,
		Mode:         imbot.ModeAuto,
		Interactions: builder.Build(),
		Timeout:      5 * time.Minute,
	}

	resp, err := h.RequestInteraction(ctx, hCtx, req)
	if err != nil {
		return -1, nil, err
	}

	// Find the selected index
	for i, opt := range options {
		if opt.Value == resp.Action.Value {
			return i, &resp.Action, nil
		}
	}

	return -1, &resp.Action, nil
}
