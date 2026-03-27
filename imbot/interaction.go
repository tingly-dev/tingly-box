package imbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/interaction"
	"github.com/tingly-dev/tingly-box/imbot/platform/dingtalk"
	"github.com/tingly-dev/tingly-box/imbot/platform/discord"
	"github.com/tingly-dev/tingly-box/imbot/platform/feishu"
	"github.com/tingly-dev/tingly-box/imbot/platform/telegram"
)

// Handler manages interaction requests and responses
type Handler struct {
	mu            sync.RWMutex
	adapters      map[core.Platform]Adapter
	pending       map[string]*PendingRequest
	botManager    BotManager
	defaultMode   InteractionMode
	pendingExpiry time.Duration
}

// BotManager is an interface for getting bots by UUID/platform
// This allows the Handler to work with different manager implementations
type BotManager interface {
	GetBot(botUUID string, platform core.Platform) core.Bot
}

// PendingRequest represents a pending interaction request
type PendingRequest struct {
	Request      InteractionRequest
	ChatID       string
	Platform     core.Platform
	BotUUID      string
	MessageID    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Mode         InteractionMode // The mode actually used for this request
	Interactions []Interaction   // Original interactions for parsing text responses
	ResponseCh   chan InteractionResponse
}

// IsExpired returns true if the pending request has expired
func (p *PendingRequest) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// NewHandler creates a new interaction handler
func NewHandler(manager BotManager) *Handler {
	h := &Handler{
		adapters:      make(map[core.Platform]Adapter),
		pending:       make(map[string]*PendingRequest),
		botManager:    manager,
		defaultMode:   ModeAuto,
		pendingExpiry: 5 * time.Minute,
	}

	// Register default adapters
	h.RegisterAdapter(core.PlatformTelegram, NewTelegramAdapter())
	h.RegisterAdapter(core.PlatformDingTalk, NewDingTalkAdapter())
	h.RegisterAdapter(core.PlatformDiscord, NewDiscordAdapter())
	h.RegisterAdapter(core.PlatformFeishu, NewFeishuAdapter())

	return h
}

// SetDefaultMode sets the global default interaction mode
func (h *Handler) SetDefaultMode(mode InteractionMode) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.defaultMode = mode
}

// SetPendingExpiry sets how long pending requests remain valid
func (h *Handler) SetPendingExpiry(duration time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pendingExpiry = duration
}

// RegisterAdapter registers an adapter for a platform
func (h *Handler) RegisterAdapter(platform core.Platform, adapter Adapter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.adapters[platform] = adapter
}

// GetAdapter returns the adapter for a platform
func (h *Handler) GetAdapter(platform core.Platform) (Adapter, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	adapter, ok := h.adapters[platform]
	return adapter, ok
}

// shouldUseInteractive determines if interactive mode should be used
// based on the requested mode and platform capabilities
func (h *Handler) shouldUseInteractive(mode InteractionMode, platform core.Platform) bool {
	h.mu.RLock()
	adapter := h.adapters[platform]
	h.mu.RUnlock()

	if adapter == nil {
		return false
	}

	switch mode {
	case ModeInteractive:
		// Force interactive - error if not supported
		return adapter.SupportsInteractions()
	case ModeText:
		// Force text mode - never use interactions
		return false
	case ModeAuto:
		// Auto-detect: use interactive if available
		return adapter.SupportsInteractions()
	default:
		return false
	}
}

// RequestInteraction sends an interaction request to the user
func (h *Handler) RequestInteraction(ctx context.Context, req InteractionRequest) (*InteractionResponse, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Determine effective mode (request mode overrides default)
	mode := req.Mode
	if mode == "" {
		mode = h.defaultMode
	}
	if mode == "" {
		mode = ModeAuto
	}

	// Get bot
	bot := h.botManager.GetBot(req.BotUUID, req.Platform)
	if bot == nil {
		return nil, ErrBotNotFound
	}

	// Get adapter
	adapter, ok := h.GetAdapter(req.Platform)
	if !ok {
		return nil, ErrNoAdapter
	}

	// Check if we should use interactive mode
	useInteractive := h.shouldUseInteractive(mode, req.Platform)

	// Validate mode compatibility
	if mode == ModeInteractive && !useInteractive {
		return nil, fmt.Errorf("%w: platform %s does not support interactive mode", ErrInvalidMode, req.Platform)
	}

	// Create response channel
	responseCh := make(chan InteractionResponse, 1)

	// Store pending request
	pending := &PendingRequest{
		Request:      req,
		Interactions: req.Interactions,
		Mode:         mode,
		ResponseCh:   responseCh,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(h.pendingExpiry),
	}

	h.mu.Lock()
	h.pending[req.ID] = pending
	h.mu.Unlock()

	// Ensure cleanup
	defer h.cleanup(req.ID)

	// Build and send message based on mode
	var opts *core.SendMessageOptions

	if useInteractive {
		// Use native interactive elements
		markup, err := adapter.BuildMarkup(req.Interactions)
		if err != nil {
			return nil, fmt.Errorf("build markup: %w", err)
		}
		opts = &core.SendMessageOptions{
			Text:      req.Message,
			ParseMode: req.ParseMode,
			Metadata:  map[string]any{"replyMarkup": markup},
		}
	} else {
		// Use text-based numbered replies (works on all platforms)
		text := adapter.BuildFallbackText(req.Message, req.Interactions)
		opts = &core.SendMessageOptions{
			Text:      text,
			ParseMode: req.ParseMode,
		}
	}

	result, err := bot.SendMessage(ctx, req.ChatID, opts)
	if err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}

	h.mu.Lock()
	if p, ok := h.pending[req.ID]; ok {
		p.MessageID = result.MessageID
		p.ChatID = req.ChatID
		p.Platform = req.Platform
		p.BotUUID = req.BotUUID
	}
	h.mu.Unlock()

	// Wait for response
	select {
	case resp := <-responseCh:
		// Update message to show result (if platform supports editing)
		if adapter.CanEditMessages() && pending.MessageID != "" {
			resultText := req.Message + "\n\n"
			if resp.IsCancel() {
				resultText += "❌ Cancelled"
			} else {
				resultText += "✅ Done"
			}
			_ = adapter.UpdateMessage(ctx, bot, req.ChatID, pending.MessageID, resultText, nil)
		}
		return &resp, nil

	case <-time.After(req.Timeout):
		// Handle timeout
		if adapter.CanEditMessages() && pending.MessageID != "" {
			_ = adapter.UpdateMessage(ctx, bot, req.ChatID, pending.MessageID,
				req.Message+"\n\n⏰ Timed out", nil)
		}
		return nil, interaction.ErrTimeout

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandleMessage handles incoming messages for interaction responses
// This should be called from the main message handler
func (h *Handler) HandleMessage(msg core.Message) (*InteractionResponse, error) {
	platform := msg.Platform
	adapter, ok := h.GetAdapter(platform)
	if !ok {
		return nil, ErrNoAdapter
	}

	// Try to parse as interaction response
	resp, err := adapter.ParseResponse(msg)
	if err != nil {
		return nil, err
	}

	// For text-based mode, look up pending request by chatID and parse numbered reply
	if resp != nil && resp.RequestID == "" {
		resp = h.parseTextResponse(msg)
		if resp == nil {
			return nil, ErrNotInteraction
		}
	}

	if resp == nil {
		return nil, ErrNotInteraction
	}

	// Deliver to pending request
	h.mu.RLock()
	pending, ok := h.pending[resp.RequestID]
	h.mu.RUnlock()

	if !ok {
		return nil, ErrRequestNotFound
	}

	// Check expiry
	if pending.IsExpired() {
		h.mu.Lock()
		delete(h.pending, resp.RequestID)
		h.mu.Unlock()
		return nil, ErrRequestExpired
	}

	select {
	case pending.ResponseCh <- *resp:
		return resp, nil
	default:
		return nil, ErrChannelClosed
	}
}

// parseTextResponse handles numbered text replies
// Works for both:
// - Platforms in text mode (Telegram with ModeText)
// - Platforms without native support (DingTalk always)
func (h *Handler) parseTextResponse(msg core.Message) *InteractionResponse {
	// Extract text from content
	var text string
	if msg.Content != nil {
		if tc, ok := msg.Content.(*core.TextContent); ok {
			text = tc.Text
		}
	}

	text = strings.TrimSpace(text)
	num, err := strconv.Atoi(text)
	if err != nil || num < 0 {
		return nil
	}

	chatID := msg.Recipient.ID

	h.mu.RLock()
	defer h.mu.RUnlock()

	// Find pending request for this chat
	for requestID, pending := range h.pending {
		if pending.ChatID == chatID && !pending.IsExpired() {
			// Cancel (0)
			if num == 0 {
				return &InteractionResponse{
					RequestID: requestID,
					Action: Interaction{
						Type:  ActionCancel,
						Value: "cancel",
					},
					Timestamp: time.Now(),
				}
			}

			// Map number to interaction (1-indexed)
			if num > 0 && num <= len(pending.Interactions) {
				interaction := pending.Interactions[num-1]
				return &InteractionResponse{
					RequestID: requestID,
					Action:    interaction,
					Timestamp: time.Now(),
				}
			}
		}
	}

	return nil
}

// cleanup removes a pending request and closes its response channel
func (h *Handler) cleanup(requestID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if pending, ok := h.pending[requestID]; ok {
		delete(h.pending, requestID)
		close(pending.ResponseCh)
	}
}

// CleanupExpired removes all expired pending requests
// Should be called periodically (e.g., via a ticker)
func (h *Handler) CleanupExpired() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	expired := make([]string, 0)
	for id, pending := range h.pending {
		if pending.IsExpired() {
			expired = append(expired, id)
		}
	}

	for _, id := range expired {
		if pending, ok := h.pending[id]; ok {
			close(pending.ResponseCh)
		}
		delete(h.pending, id)
	}

	return len(expired)
}

// GetPendingRequest returns a pending request by ID
func (h *Handler) GetPendingRequest(requestID string) (*PendingRequest, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pending, ok := h.pending[requestID]
	if !ok || pending.IsExpired() {
		return nil, false
	}
	return pending, true
}

// GetPendingRequestsForChat returns all pending requests for a specific chat
func (h *Handler) GetPendingRequestsForChat(chatID string) []InteractionRequest {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var requests []InteractionRequest
	for _, pending := range h.pending {
		if pending.ChatID == chatID && !pending.IsExpired() {
			requests = append(requests, pending.Request)
		}
	}
	return requests
}

// CancelRequest cancels a pending request
func (h *Handler) CancelRequest(requestID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	pending, ok := h.pending[requestID]
	if !ok {
		return ErrRequestNotFound
	}

	// Send cancellation response
	select {
	case pending.ResponseCh <- InteractionResponse{
		RequestID: requestID,
		Action: Interaction{
			Type:  ActionCancel,
			Value: "cancelled",
		},
		Timestamp: time.Now(),
	}:
	default:
	}

	close(pending.ResponseCh)
	delete(h.pending, requestID)
	return nil
}

// SubmitResponse submits a response for a pending request
// This is useful for testing or programmatic responses
func (h *Handler) SubmitResponse(requestID string, response InteractionResponse) error {
	h.mu.RLock()
	pending, ok := h.pending[requestID]
	h.mu.RUnlock()

	if !ok {
		return ErrRequestNotFound
	}

	if pending.IsExpired() {
		return ErrRequestExpired
	}

	select {
	case pending.ResponseCh <- response:
		return nil
	default:
		return ErrChannelClosed
	}
}

// Platform adapter constructors

// NewTelegramAdapter creates a new Telegram interaction adapter
func NewTelegramAdapter() Adapter {
	return telegram.NewInteractionAdapter()
}

// NewDingTalkAdapter creates a new DingTalk interaction adapter
func NewDingTalkAdapter() Adapter {
	return dingtalk.NewInteractionAdapter()
}

// NewDiscordAdapter creates a new Discord interaction adapter
func NewDiscordAdapter() Adapter {
	return discord.NewInteractionAdapter()
}

// NewFeishuAdapter creates a new Feishu interaction adapter
func NewFeishuAdapter() Adapter {
	return feishu.NewInteractionAdapter()
}
