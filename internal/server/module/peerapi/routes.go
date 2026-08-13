package peerapi

import (
	"github.com/tingly-dev/tingly-box/swagger"
)

// PeerView is the API projection of a Peer. The token hash is never
// serialized; Online reports whether a poller is connected now.
type PeerView struct {
	UUID      string `json:"uuid" example:"3d5a…"`
	Name      string `json:"name" example:"report"`
	BotUUID   string `json:"bot_uuid" example:"bot-uuid"`
	ChatID    string `json:"chat_id" example:"123456789"`
	Exclusive bool   `json:"exclusive" example:"false"`
	Enabled   bool   `json:"enabled" example:"true"`
	Online    bool   `json:"online" example:"false"`
	CreatedAt string `json:"created_at" example:"2026-08-13T12:00:00Z"`
	UpdatedAt string `json:"updated_at" example:"2026-08-13T12:00:00Z"`
}

// CreatePeerRequest is the body of POST /peers.
type CreatePeerRequest struct {
	Name      string `json:"name" binding:"required" example:"report"`
	BotUUID   string `json:"bot_uuid" binding:"required" example:"bot-uuid"`
	ChatID    string `json:"chat_id" binding:"required" example:"123456789"`
	Exclusive bool   `json:"exclusive,omitempty" example:"false"`
	Enabled   *bool  `json:"enabled,omitempty" example:"true"`
}

// CreatePeerResponse carries the one-time plaintext token.
type CreatePeerResponse struct {
	Peer  PeerView `json:"peer"`
	Token string   `json:"token" example:"tb-peer-…"`
}

// UpdatePeerRequest is the partial body of PUT /peers/:id.
type UpdatePeerRequest struct {
	Name      *string `json:"name,omitempty" example:"report"`
	BotUUID   *string `json:"bot_uuid,omitempty" example:"bot-uuid"`
	ChatID    *string `json:"chat_id,omitempty" example:"123456789"`
	Exclusive *bool   `json:"exclusive,omitempty" example:"true"`
	Enabled   *bool   `json:"enabled,omitempty" example:"false"`
}

// PeerResponse wraps one peer.
type PeerResponse struct {
	Peer PeerView `json:"peer"`
}

// PeerListResponse wraps the list.
type PeerListResponse struct {
	Peers []PeerView `json:"peers"`
}

// RotateTokenResponse carries the one-time replacement token.
type RotateTokenResponse struct {
	Token string `json:"token" example:"tb-peer-…"`
}

// SendRequest is the body of POST /peers/:id/send — the one outbound verb.
// The target chat is the peer's binding; there is no target field by design.
type SendRequest struct {
	Text string `json:"text" binding:"required" example:"job 7 restarted ✅"`
	// ReplyToUpdateID threads the message to the referenced inbound update's
	// original chat message when given.
	ReplyToUpdateID int64 `json:"reply_to_update_id,omitempty" example:"42"`
}

// SendResponse reports the delivered message.
type SendResponse struct {
	OK bool `json:"ok" example:"true"`
	// MessageID is the platform message id of the sent message, when the
	// platform reports one.
	MessageID string `json:"message_id,omitempty" example:"1024"`
}

// UpdateView is one entry of the inbound stream: a typed envelope
// (type "message" in v1; new types may appear — ignore unknown ones).
type UpdateView struct {
	UpdateID  int64  `json:"update_id" example:"42"`
	Type      string `json:"type" example:"message"`
	ChatID    string `json:"chat_id" example:"123456789"`
	SenderID  string `json:"sender_id" example:"987654321"`
	MessageID string `json:"message_id,omitempty" example:"1024"`
	Text      string `json:"text" example:"run job 7"`
	CreatedAt string `json:"created_at" example:"2026-08-13T12:00:00Z"`
}

// UpdatesResponse is the body of GET /peers/:id/updates.
type UpdatesResponse struct {
	Updates []UpdateView `json:"updates"`
}

// OKResponse is the generic mutation result.
type OKResponse struct {
	OK bool `json:"ok" example:"true"`
}

// RegisterControlRoutes registers the CRUD + token surface on the operator
// control-plane group (the existing apiV1 group with UserAuth middleware).
func RegisterControlRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.GET("/peers", handler.List,
		swagger.WithTags("peer"),
		swagger.WithDescription("List peers (no tokens)."),
		swagger.WithResponseModel(PeerListResponse{}))

	router.POST("/peers", handler.Create,
		swagger.WithTags("peer"),
		swagger.WithDescription("Register a peer; the scoped tb-peer- token is returned exactly once."),
		swagger.WithRequestModel(CreatePeerRequest{}),
		swagger.WithResponseModel(CreatePeerResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid name or missing fields"},
			swagger.ErrorResponseConfig{Code: 409, Message: "Name already taken"},
		))

	router.GET("/peers/:id", handler.Get,
		swagger.WithTags("peer"),
		swagger.WithDescription("Get one peer."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithResponseModel(PeerResponse{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: 404, Message: "Not found"}))

	router.PUT("/peers/:id", handler.Update,
		swagger.WithTags("peer"),
		swagger.WithDescription("Update name/bot/chat/exclusive/enabled (partial)."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithRequestModel(UpdatePeerRequest{}),
		swagger.WithResponseModel(PeerResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "Not found"},
			swagger.ErrorResponseConfig{Code: 409, Message: "Name already taken"},
		))

	router.DELETE("/peers/:id", handler.Delete,
		swagger.WithTags("peer"),
		swagger.WithDescription("Delete the peer and its queued updates."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithResponseModel(OKResponse{}))

	router.POST("/peers/:id/token", handler.RotateToken,
		swagger.WithTags("peer"),
		swagger.WithDescription("Rotate the scoped token; the old one stops working immediately."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithResponseModel(RotateTokenResponse{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: 404, Message: "Not found"}))
}

// RegisterDataRoutes registers the tool-facing data plane on a group guarded
// by DataAuthMiddleware (peer token or operator token). Two verbs — the
// whole protocol (spec §5).
func RegisterDataRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.POST("/peers/:id/send", handler.Send,
		swagger.WithTags("peer-data"),
		swagger.WithDescription("Send a message into the peer's bound chat, attributed with its name; reply_to_update_id threads it to an inbound update. Accepts the peer's tb-peer- token."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithRequestModel(SendRequest{}),
		swagger.WithResponseModel(SendResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 401, Message: "Invalid token"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Peer not available or bot not running"},
		))

	router.GET("/peers/:id/updates", handler.Updates,
		swagger.WithTags("peer-data"),
		swagger.WithDescription("Long-poll the inbound stream (getUpdates semantics): offset confirms every update below it and returns the rest, oldest first; omit offset to re-read unconfirmed updates."),
		swagger.WithPathParam("id", "string", "Peer UUID"),
		swagger.WithResponseModel(UpdatesResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 401, Message: "Invalid token"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Peer not available"},
		))
}
