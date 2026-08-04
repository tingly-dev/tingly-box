// Package subscriptionapi is the HTTP surface of the Subscription resource
// (.design/subscription.md): control-plane CRUD + token rotation behind the
// operator UserToken, and the tool-facing data plane (notify / interact /
// events / reply) behind the scoped tb-sub- token.
//
// Delivery reuses the same channel.Registry + interaction.Registry the bot
// interaction API drives — no new runtime; the subscription is a caller
// identity and a chat scope on top of the existing message machinery.
package subscriptionapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/subscription"
)

// Interact budget bounds, aligned with the bot interaction API.
const (
	defaultInteractTimeout = 5 * time.Minute
	maxInteractTimeout     = 30 * time.Minute
)

// Event long-poll bounds.
const (
	defaultEventsTimeout = 25 * time.Second
	maxEventsTimeout     = 60 * time.Second
	defaultEventsLimit   = 100
	maxEventsLimit       = 500
)

// trackedSender is the optional channel capability that returns the platform
// message id of a sent message; imchannel.Channel implements it. Without it
// reply-to addressing simply doesn't accrue state for this bot's platform.
type trackedSender interface {
	SendTracked(ctx context.Context, target channel.Target, msg interaction.Notification) (string, error)
}

// Handler serves the subscription API.
type Handler struct {
	store    subscription.Store
	mailbox  *subscription.Mailbox
	sends    *subscription.RecentSends
	channels *channel.Registry
	results  *interaction.Registry[interaction.Result]
}

// NewHandler builds the handler. mailbox and sends MUST be the same
// instances the inbound consumer uses (imbot.SubscriptionRuntime).
func NewHandler(store subscription.Store, mailbox *subscription.Mailbox, sends *subscription.RecentSends, channels *channel.Registry, results *interaction.Registry[interaction.Result]) *Handler {
	return &Handler{store: store, mailbox: mailbox, sends: sends, channels: channels, results: results}
}

// ---- control plane ----

// List handles GET /api/v1/subscriptions.
func (h *Handler) List(c *gin.Context) {
	subs, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}
	views := make([]SubscriptionView, 0, len(subs))
	for _, sub := range subs {
		views = append(views, h.view(sub))
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": views})
}

// Create handles POST /api/v1/subscriptions. The scoped token is returned
// exactly once, here (and on rotate).
func (h *Handler) Create(c *gin.Context) {
	var req CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if err := subscription.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.BotUUID == "" || req.ChatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_uuid and chat_id are required"})
		return
	}

	token, hash, err := subscription.NewToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token mint failed"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sub := subscription.Subscription{
		Name:      req.Name,
		BotUUID:   req.BotUUID,
		ChatID:    req.ChatID,
		Exclusive: req.Exclusive,
		Enabled:   enabled,
		TokenHash: hash,
	}
	if err := h.store.Create(&sub); err != nil {
		if errors.Is(err, subscription.ErrNameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "name already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}
	h.audit(sub.UUID, "subscription.create", map[string]any{"name": sub.Name, "bot": sub.BotUUID})
	c.JSON(http.StatusCreated, gin.H{"subscription": h.view(sub), "token": token})
}

// Get handles GET /api/v1/subscriptions/:id.
func (h *Handler) Get(c *gin.Context) {
	sub, ok := h.lookup(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscription": h.view(sub)})
}

// Update handles PUT /api/v1/subscriptions/:id (partial).
func (h *Handler) Update(c *gin.Context) {
	sub, ok := h.lookup(c)
	if !ok {
		return
	}
	var req UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if req.Name != nil {
		if err := subscription.ValidateName(*req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sub.Name = *req.Name
	}
	if req.BotUUID != nil {
		sub.BotUUID = *req.BotUUID
	}
	if req.ChatID != nil {
		sub.ChatID = *req.ChatID
	}
	if req.Exclusive != nil {
		sub.Exclusive = *req.Exclusive
	}
	if req.Enabled != nil {
		sub.Enabled = *req.Enabled
	}
	if err := h.store.Update(&sub); err != nil {
		switch {
		case errors.Is(err, subscription.ErrNameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": "name already taken"})
		case errors.Is(err, subscription.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		}
		return
	}
	h.audit(sub.UUID, "subscription.update", nil)
	c.JSON(http.StatusOK, gin.H{"subscription": h.view(sub)})
}

// Delete handles DELETE /api/v1/subscriptions/:id.
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	h.audit(id, "subscription.delete", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RotateToken handles POST /api/v1/subscriptions/:id/token. The old token
// stops working immediately; the new one is returned exactly once.
func (h *Handler) RotateToken(c *gin.Context) {
	sub, ok := h.lookup(c)
	if !ok {
		return
	}
	token, hash, err := subscription.NewToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token mint failed"})
		return
	}
	sub.TokenHash = hash
	if err := h.store.Update(&sub); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rotate failed"})
		return
	}
	h.audit(sub.UUID, "subscription.token.rotate", nil)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ---- data plane ----

// Notify handles POST /api/v1/subscriptions/:id/notify — a one-way push into
// the bound chat, attributed with the subscription's name.
func (h *Handler) Notify(c *gin.Context) {
	sub, ch, ok := h.deliverable(c)
	if !ok {
		return
	}
	var req NotifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	notification := attributed(sub, req.Title, req.Body)
	if req.Level != "" {
		if notification.Meta == nil {
			notification.Meta = map[string]any{}
		}
		notification.Meta["level"] = req.Level
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.send(ctx, sub, ch, notification); err != nil {
		h.audit(sub.UUID, "subscription.notify.error", map[string]any{"err": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delivery failed"})
		return
	}
	h.audit(sub.UUID, "subscription.notify", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Interact handles POST /api/v1/subscriptions/:id/interact — starts an
// interactive prompt in the bound chat and returns a request id to long-poll.
func (h *Handler) Interact(c *gin.Context) {
	if h.results == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interaction registry unavailable"})
		return
	}
	sub, ch, ok := h.deliverable(c)
	if !ok {
		return
	}
	var req InteractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	kind := interaction.Kind(req.Kind)
	switch kind {
	case interaction.KindConfirm, interaction.KindChoose:
		if len(req.Options) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("kind %q requires at least one option", req.Kind)})
			return
		}
	case interaction.KindAsk:
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid kind %q (want confirm|choose|ask)", req.Kind)})
		return
	}

	budget := interactBudget(req.TimeoutSeconds)
	id := newRequestID()
	if !h.results.Begin(id) {
		c.JSON(http.StatusConflict, gin.H{"error": "request id collision, retry"})
		return
	}

	ix := interaction.Interaction{
		ID:      id,
		Kind:    kind,
		Title:   sub.AttributionPrefix() + req.Title,
		Body:    req.Body,
		Options: req.Options,
		Timeout: budget,
		// Same reply metadata contract as the bot interaction API: the host
		// authorization gate reads these off the pending request.
		Meta: map[string]any{"capability": "notify", "reply_action": "notify.reply"},
	}
	go h.runPrompt(sub, ch, ix, budget)

	h.audit(sub.UUID, "subscription.interact.start", map[string]any{"interaction_id": id, "kind": string(kind)})
	c.JSON(http.StatusAccepted, gin.H{
		"request_id": id,
		"wait_url":   fmt.Sprintf("/api/v1/subscriptions/%s/interact/%s", sub.UUID, id),
		"expires_at": time.Now().Add(budget).UTC().Format(time.RFC3339),
	})
}

// Wait handles GET /api/v1/subscriptions/:id/interact/:request_id — the same
// 200/410/504/404 matrix as the bot interaction API.
func (h *Handler) Wait(c *gin.Context) {
	if h.results == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	requestID := c.Param("request_id")
	resultCh, ok := h.results.Await(requestID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"status": "expired"})
		return
	}
	timeout := parseWaitTimeout(c.Query("timeout"))
	select {
	case res := <-resultCh:
		switch res.Status {
		case interaction.StatusAnswered, interaction.StatusCancelled:
			c.JSON(http.StatusOK, gin.H{"status": string(res.Status), "decision": res.Decision})
		case interaction.StatusTimeout:
			c.JSON(http.StatusGone, gin.H{"status": "timeout"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"status": string(res.Status), "reason": res.Reason})
		}
	case <-time.After(timeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	case <-c.Request.Context().Done():
		c.JSON(http.StatusGatewayTimeout, gin.H{"status": "pending"})
	}
}

// Events handles GET /api/v1/subscriptions/:id/events — the inbound mailbox
// long-poll. Returns events past the acked cursor, oldest first; the caller
// acks by POSTing the last id to /events/ack.
func (h *Handler) Events(c *gin.Context) {
	sub, ok := h.lookupEnabled(c)
	if !ok {
		return
	}
	timeout := parseEventsTimeout(c.Query("timeout"))
	limit := parseEventsLimit(c.Query("limit"))

	events, err := h.mailbox.Poll(c.Request.Context(), sub.UUID, timeout, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.JSON(http.StatusGatewayTimeout, gin.H{"events": []EventView{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "poll failed"})
		return
	}
	views := make([]EventView, 0, len(events))
	for _, ev := range events {
		views = append(views, EventView{
			ID:        ev.ID,
			ChatID:    ev.ChatID,
			SenderID:  ev.SenderID,
			MessageID: ev.MessageID,
			Text:      ev.Text,
			CreatedAt: ev.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"events": views})
}

// AckEvents handles POST /api/v1/subscriptions/:id/events/ack.
func (h *Handler) AckEvents(c *gin.Context) {
	sub, ok := h.lookupEnabled(c)
	if !ok {
		return
	}
	var req AckRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UpTo <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "up_to (positive event id) is required"})
		return
	}
	if err := h.mailbox.Ack(sub.UUID, req.UpTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ack failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reply handles POST /api/v1/subscriptions/:id/reply — sends into the bound
// chat, threaded to the referenced event's message when event_id is given.
func (h *Handler) Reply(c *gin.Context) {
	sub, ch, ok := h.deliverable(c)
	if !ok {
		return
	}
	var req ReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	notification := attributed(sub, "", req.Text)
	if req.EventID > 0 {
		// Best-effort threading: an already-pruned event just sends
		// unthreaded rather than failing the reply.
		if ev, err := h.store.GetEvent(sub.UUID, req.EventID); err == nil {
			notification.Meta = map[string]any{}
			if ev.MessageID != "" {
				notification.Meta["reply_to"] = ev.MessageID
			}
			if ev.ContextToken != "" {
				notification.Meta["context_token"] = ev.ContextToken
			}
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.send(ctx, sub, ch, notification); err != nil {
		h.audit(sub.UUID, "subscription.reply.error", map[string]any{"err": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delivery failed"})
		return
	}
	h.audit(sub.UUID, "subscription.reply", map[string]any{"event_id": req.EventID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---- helpers ----

func (h *Handler) view(sub subscription.Subscription) SubscriptionView {
	online := h.mailbox != nil && h.mailbox.HasWaiter(sub.UUID)
	return SubscriptionView{
		UUID:      sub.UUID,
		Name:      sub.Name,
		BotUUID:   sub.BotUUID,
		ChatID:    sub.ChatID,
		Exclusive: sub.Exclusive,
		Enabled:   sub.Enabled,
		Online:    online,
		CreatedAt: sub.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: sub.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) lookup(c *gin.Context) (subscription.Subscription, bool) {
	sub, err := h.store.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return subscription.Subscription{}, false
	}
	return sub, true
}

// lookupEnabled is the data-plane variant: a disabled subscription is
// reported with the same body as a missing one so the data plane doesn't
// leak state.
func (h *Handler) lookupEnabled(c *gin.Context) (subscription.Subscription, bool) {
	sub, err := h.store.Get(c.Param("id"))
	if err != nil || !sub.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not available"})
		return subscription.Subscription{}, false
	}
	return sub, true
}

// deliverable resolves an enabled subscription AND its bot's live channel.
// Unknown, disabled, and stopped-bot all surface as 404 (uniform body per
// endpoint class, mirroring the bot API's defend-in-depth rule).
func (h *Handler) deliverable(c *gin.Context) (subscription.Subscription, channel.Channel, bool) {
	sub, ok := h.lookupEnabled(c)
	if !ok {
		return subscription.Subscription{}, nil, false
	}
	if h.channels == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not running"})
		return subscription.Subscription{}, nil, false
	}
	ch, ok := h.channels.Get(sub.BotUUID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not running"})
		return subscription.Subscription{}, nil, false
	}
	return sub, ch, true
}

// send delivers a notification into the subscription's bound chat, tracking
// the sent message id for reply-to addressing when the channel supports it.
func (h *Handler) send(ctx context.Context, sub subscription.Subscription, ch channel.Channel, msg interaction.Notification) error {
	target := channel.Target{ChatID: sub.ChatID}
	if tracked, ok := ch.(trackedSender); ok {
		messageID, err := tracked.SendTracked(ctx, target, msg)
		if err != nil {
			return err
		}
		if h.sends != nil {
			h.sends.Track(sub.ChatID, messageID, sub.UUID)
		}
		return nil
	}
	return ch.Send(ctx, target, msg)
}

func (h *Handler) runPrompt(sub subscription.Subscription, ch channel.Channel, ix interaction.Interaction, budget time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	reply, err := ch.Prompt(ctx, channel.Target{ChatID: sub.ChatID}, ix)
	if err != nil {
		status := interaction.StatusError
		if errors.Is(err, context.DeadlineExceeded) {
			status = interaction.StatusTimeout
		}
		h.results.Resolve(ix.ID, interaction.Result{Status: status, Reason: err.Error()})
		h.audit(sub.UUID, "subscription.interact.error", map[string]any{"interaction_id": ix.ID, "err": err.Error()})
		return
	}
	status := interaction.StatusAnswered
	if reply.Status == interaction.StatusCancelled {
		status = interaction.StatusCancelled
	}
	decision := map[string]any{}
	if reply.Selected != "" {
		decision["selected"] = reply.Selected
	}
	if reply.FreeText != "" {
		decision["free_text"] = reply.FreeText
	}
	h.results.Resolve(ix.ID, interaction.Result{Status: status, Decision: decision})
	h.audit(sub.UUID, "subscription.interact.done", map[string]any{"interaction_id": ix.ID, "status": string(status)})
}

// attributed builds the outbound notification with the 【name】 prefix on the
// title, or on the body when there is no title — two tools sharing a chat
// must be distinguishable.
func attributed(sub subscription.Subscription, title, body string) interaction.Notification {
	if title != "" {
		return interaction.Notification{Title: sub.AttributionPrefix() + title, Body: body}
	}
	return interaction.Notification{Body: sub.AttributionPrefix() + " " + body}
}

func (h *Handler) audit(subUUID, action string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["subscription"] = subUUID
	logrus.WithFields(logrus.Fields(details)).WithField("action", action).Info(action)
}

func interactBudget(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultInteractTimeout
	}
	d := time.Duration(seconds) * time.Second
	if d > maxInteractTimeout {
		return maxInteractTimeout
	}
	return d
}

func parseWaitTimeout(raw string) time.Duration {
	if raw == "" {
		return 45 * time.Second
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 && d <= 5*time.Minute {
		return d
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 && secs <= 300 {
		return time.Duration(secs) * time.Second
	}
	return 45 * time.Second
}

func parseEventsTimeout(raw string) time.Duration {
	if raw == "" {
		return defaultEventsTimeout
	}
	var d time.Duration
	if parsed, err := time.ParseDuration(raw); err == nil {
		d = parsed
	} else if secs, err := strconv.Atoi(raw); err == nil {
		d = time.Duration(secs) * time.Second
	} else {
		return defaultEventsTimeout
	}
	if d < 0 {
		return 0
	}
	if d > maxEventsTimeout {
		return maxEventsTimeout
	}
	return d
}

func parseEventsLimit(raw string) int {
	if raw == "" {
		return defaultEventsLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultEventsLimit
	}
	if n > maxEventsLimit {
		return maxEventsLimit
	}
	return n
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
