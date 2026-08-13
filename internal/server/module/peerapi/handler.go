// Package peerapi is the HTTP surface of the Peer resource (.design/peer.md):
// control-plane CRUD + token rotation behind the operator UserToken, and the
// tool-facing data plane — the two IM-platform verbs, send and updates —
// behind the scoped tb-peer- token.
//
// Delivery reuses the channel.Registry the bot API drives — no new runtime;
// the peer is a caller identity and a chat scope on top of the existing
// message machinery.
package peerapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/peer"
)

// Updates long-poll bounds.
const (
	defaultUpdatesTimeout = 25 * time.Second
	maxUpdatesTimeout     = 60 * time.Second
	defaultUpdatesLimit   = 100
	maxUpdatesLimit       = 500
)

// trackedSender is the optional channel capability that returns the platform
// message id of a sent message; imchannel.Channel implements it. Without it
// reply-to addressing simply doesn't accrue state for this bot's platform.
type trackedSender interface {
	SendTracked(ctx context.Context, target channel.Target, msg interaction.Notification) (string, error)
}

// Handler serves the peer API.
type Handler struct {
	store    peer.Store
	inbox    *peer.Inbox
	sends    *peer.RecentSends
	channels *channel.Registry
}

// NewHandler builds the handler. inbox and sends MUST be the same instances
// the inbound consumer uses (imbot.PeerRuntime).
func NewHandler(store peer.Store, inbox *peer.Inbox, sends *peer.RecentSends, channels *channel.Registry) *Handler {
	return &Handler{store: store, inbox: inbox, sends: sends, channels: channels}
}

// ---- control plane ----

// List handles GET /api/v1/peers.
func (h *Handler) List(c *gin.Context) {
	peers, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		return
	}
	views := make([]PeerView, 0, len(peers))
	for _, p := range peers {
		views = append(views, h.view(p))
	}
	c.JSON(http.StatusOK, gin.H{"peers": views})
}

// Create handles POST /api/v1/peers. The scoped token is returned exactly
// once, here (and on rotate).
func (h *Handler) Create(c *gin.Context) {
	var req CreatePeerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if err := peer.ValidateName(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.BotUUID == "" || req.ChatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot_uuid and chat_id are required"})
		return
	}

	token, hash, err := peer.NewToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token mint failed"})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	p := peer.Peer{
		Name:      req.Name,
		BotUUID:   req.BotUUID,
		ChatID:    req.ChatID,
		Exclusive: req.Exclusive,
		Enabled:   enabled,
		TokenHash: hash,
	}
	if err := h.store.Create(&p); err != nil {
		if errors.Is(err, peer.ErrNameTaken) {
			c.JSON(http.StatusConflict, gin.H{"error": "name already taken"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create failed"})
		return
	}
	h.audit(p.UUID, "peer.create", map[string]any{"name": p.Name, "bot": p.BotUUID})
	c.JSON(http.StatusCreated, gin.H{"peer": h.view(p), "token": token})
}

// Get handles GET /api/v1/peers/:id.
func (h *Handler) Get(c *gin.Context) {
	p, ok := h.lookup(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"peer": h.view(p)})
}

// Update handles PUT /api/v1/peers/:id (partial).
func (h *Handler) Update(c *gin.Context) {
	p, ok := h.lookup(c)
	if !ok {
		return
	}
	var req UpdatePeerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if req.Name != nil {
		if err := peer.ValidateName(*req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		p.Name = *req.Name
	}
	if req.BotUUID != nil {
		p.BotUUID = *req.BotUUID
	}
	if req.ChatID != nil {
		p.ChatID = *req.ChatID
	}
	if req.Exclusive != nil {
		p.Exclusive = *req.Exclusive
	}
	if req.Enabled != nil {
		p.Enabled = *req.Enabled
	}
	if err := h.store.Save(&p); err != nil {
		switch {
		case errors.Is(err, peer.ErrNameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": "name already taken"})
		case errors.Is(err, peer.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "peer not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		}
		return
	}
	h.audit(p.UUID, "peer.update", nil)
	c.JSON(http.StatusOK, gin.H{"peer": h.view(p)})
}

// Delete handles DELETE /api/v1/peers/:id.
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	h.audit(id, "peer.delete", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RotateToken handles POST /api/v1/peers/:id/token. The old token stops
// working immediately; the new one is returned exactly once.
func (h *Handler) RotateToken(c *gin.Context) {
	p, ok := h.lookup(c)
	if !ok {
		return
	}
	token, hash, err := peer.NewToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token mint failed"})
		return
	}
	p.TokenHash = hash
	if err := h.store.Save(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "rotate failed"})
		return
	}
	h.audit(p.UUID, "peer.token.rotate", nil)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

// ---- data plane: the two IM-platform verbs ----

// Send handles POST /api/v1/peers/:id/send — the one outbound verb: deliver
// a message into the bound chat, attributed with the peer's name, optionally
// threaded to an inbound update.
func (h *Handler) Send(c *gin.Context) {
	p, ch, ok := h.deliverable(c)
	if !ok {
		return
	}
	var req SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	msg := interaction.Notification{Body: p.AttributionPrefix() + " " + req.Text}
	if req.ReplyToUpdateID > 0 {
		// Best-effort threading: an already-pruned update just sends
		// unthreaded rather than failing the send.
		if u, err := h.store.GetUpdate(p.UUID, req.ReplyToUpdateID); err == nil {
			msg.Meta = map[string]any{}
			if u.MessageID != "" {
				msg.Meta["reply_to"] = u.MessageID
			}
			if u.ContextToken != "" {
				msg.Meta["context_token"] = u.ContextToken
			}
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	messageID, err := h.send(ctx, p, ch, msg)
	if err != nil {
		h.audit(p.UUID, "peer.send.error", map[string]any{"err": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delivery failed"})
		return
	}
	h.audit(p.UUID, "peer.send", map[string]any{"reply_to_update_id": req.ReplyToUpdateID})
	c.JSON(http.StatusOK, gin.H{"ok": true, "message_id": messageID})
}

// Updates handles GET /api/v1/peers/:id/updates — the inbound long-poll with
// getUpdates semantics: a positive offset confirms every update below it;
// omitting offset re-reads unconfirmed updates (crash replay).
func (h *Handler) Updates(c *gin.Context) {
	p, ok := h.lookupEnabled(c)
	if !ok {
		return
	}
	offset := parseOffset(c.Query("offset"))
	timeout := parseUpdatesTimeout(c.Query("timeout"))
	limit := parseUpdatesLimit(c.Query("limit"))

	updates, err := h.inbox.Poll(c.Request.Context(), p.UUID, offset, timeout, limit)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.JSON(http.StatusGatewayTimeout, gin.H{"updates": []UpdateView{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "poll failed"})
		return
	}
	views := make([]UpdateView, 0, len(updates))
	for _, u := range updates {
		views = append(views, UpdateView{
			UpdateID:  u.ID,
			Type:      u.Type,
			ChatID:    u.ChatID,
			SenderID:  u.SenderID,
			MessageID: u.MessageID,
			Text:      u.Text,
			CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, gin.H{"updates": views})
}

// ---- helpers ----

func (h *Handler) view(p peer.Peer) PeerView {
	online := h.inbox != nil && h.inbox.HasWaiter(p.UUID)
	return PeerView{
		UUID:      p.UUID,
		Name:      p.Name,
		BotUUID:   p.BotUUID,
		ChatID:    p.ChatID,
		Exclusive: p.Exclusive,
		Enabled:   p.Enabled,
		Online:    online,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *Handler) lookup(c *gin.Context) (peer.Peer, bool) {
	p, err := h.store.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer not found"})
		return peer.Peer{}, false
	}
	return p, true
}

// lookupEnabled is the data-plane variant: a disabled peer is reported with
// the same body as a missing one so the data plane doesn't leak state.
func (h *Handler) lookupEnabled(c *gin.Context) (peer.Peer, bool) {
	p, err := h.store.Get(c.Param("id"))
	if err != nil || !p.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer not available"})
		return peer.Peer{}, false
	}
	return p, true
}

// deliverable resolves an enabled peer AND its bot's live channel. Unknown,
// disabled, and stopped-bot all surface as 404 (uniform body per endpoint
// class, mirroring the bot API's defend-in-depth rule).
func (h *Handler) deliverable(c *gin.Context) (peer.Peer, channel.Channel, bool) {
	p, ok := h.lookupEnabled(c)
	if !ok {
		return peer.Peer{}, nil, false
	}
	if h.channels == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not running"})
		return peer.Peer{}, nil, false
	}
	ch, ok := h.channels.Get(p.BotUUID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "bot not running"})
		return peer.Peer{}, nil, false
	}
	return p, ch, true
}

// send delivers a notification into the peer's bound chat, tracking the sent
// message id for reply-to addressing when the channel supports it.
func (h *Handler) send(ctx context.Context, p peer.Peer, ch channel.Channel, msg interaction.Notification) (string, error) {
	target := channel.Target{ChatID: p.ChatID}
	if tracked, ok := ch.(trackedSender); ok {
		messageID, err := tracked.SendTracked(ctx, target, msg)
		if err != nil {
			return "", err
		}
		if h.sends != nil {
			h.sends.Track(p.ChatID, messageID, p.UUID)
		}
		return messageID, nil
	}
	return "", ch.Send(ctx, target, msg)
}

func (h *Handler) audit(peerUUID, action string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	details["peer"] = peerUUID
	logrus.WithFields(logrus.Fields(details)).WithField("action", action).Info(action)
}

func parseOffset(raw string) int64 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseUpdatesTimeout(raw string) time.Duration {
	if raw == "" {
		return defaultUpdatesTimeout
	}
	var d time.Duration
	if parsed, err := time.ParseDuration(raw); err == nil {
		d = parsed
	} else if secs, err := strconv.Atoi(raw); err == nil {
		d = time.Duration(secs) * time.Second
	} else {
		return defaultUpdatesTimeout
	}
	if d < 0 {
		return 0
	}
	if d > maxUpdatesTimeout {
		return maxUpdatesTimeout
	}
	return d
}

func parseUpdatesLimit(raw string) int {
	if raw == "" {
		return defaultUpdatesLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultUpdatesLimit
	}
	if n > maxUpdatesLimit {
		return maxUpdatesLimit
	}
	return n
}
