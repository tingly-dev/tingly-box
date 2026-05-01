package weixin

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/platform"
)

// Handler implements platform-specific handling for Weixin
type Handler struct {
	bot           imbot.Bot
	qrClient      *QRClient
	logger        *weixinLogger
}

// NewHandler creates a new Weixin platform handler
func NewHandler(bot imbot.Bot, qrClient *QRClient) *Handler {
	if qrClient == nil {
		qrClient = NewQRClient("")
	}
	return &Handler{
		bot:      bot,
		qrClient: qrClient,
		logger:   &weixinLogger{},
	}
}

// Platform returns the Weixin platform identifier
func (h *Handler) Platform() imbot.Platform {
	return "weixin"
}

// SupportsFeature checks if Weixin supports a specific feature
func (h *Handler) SupportsFeature(feature platform.Feature) bool {
	switch feature {
	case platform.FeatureQRAuth,
		platform.FeatureMediaUpload:
		return true
	case platform.FeatureVerbose,
		platform.FeatureInlineKeyboard,
		platform.FeatureMarkdown,
		platform.FeatureReactions,
		platform.FeatureMessageEditing,
		platform.FeatureKeyboardRemoval,
		platform.FeatureCardRendering:
		return false
	default:
		return false
	}
}

// HandlePlatformMessage handles Weixin-specific message preprocessing
func (h *Handler) HandlePlatformMessage(ctx *platform.Context) (bool, error) {
	// Check for QR authentication completion
	if qrStatus, ok := ctx.Message.Metadata["qr_status"].(string); ok && qrStatus == "confirmed" {
		// QR authentication completed
		return false, nil
	}

	// No special preprocessing needed for Weixin
	return false, nil
}

// SendMessage sends a message with Weixin-specific formatting
func (h *Handler) SendMessage(ctx context.Context, chatID string, opts *imbot.SendMessageOptions) error {
	// Weixin requires context_token to be forwarded from incoming messages
	// This is handled in the main send logic
	return nil
}

// GetQRCode fetches a QR code for Weixin bot login
func (h *Handler) GetQRCode(ctx context.Context, botType string) (*QRCodeResponse, error) {
	return h.qrClient.GetBotQRCode(ctx, botType)
}

// GetQRStatus polls the QR code status
func (h *Handler) GetQRStatus(ctx context.Context, qrcode string) (*QRStatusResponse, error) {
	return h.qrClient.GetQRStatus(ctx, qrcode)
}

// PollQRStatus polls the QR status until confirmed or expired
func (h *Handler) PollQRStatus(ctx context.Context, qrID string, pollInterval int) (*QRStatusResponse, error) {
	return PollQRStatus(ctx, h.qrClient, qrID, pollInterval)
}

// SupportsVerboseMode returns false since Weixin doesn't support verbose intermediate messages
func (h *Handler) SupportsVerboseMode() bool {
	return false
}

// GetParseMode returns plain text for Weixin (no markdown support)
func (h *Handler) GetParseMode() imbot.ParseMode {
	return imbot.ParseModeNone
}

// weixinLogger is a minimal logger for Weixin operations
type weixinLogger struct{}

// Debug logs debug messages
func (l *weixinLogger) Debug(args ...interface{}) {}

// Info logs info messages
func (l *weixinLogger) Info(args ...interface{}) {}

// Warn logs warning messages
func (l *weixinLogger) Warn(args ...interface{}) {}

// Error logs error messages
func (l *weixinLogger) Error(args ...interface{}) {}
