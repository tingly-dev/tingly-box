package subscriptionapi

import (
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/swagger"
)

// SubscriptionView is the API projection of a Subscription. The token hash
// is never serialized; Online reports whether a poller is connected now.
type SubscriptionView struct {
	UUID      string `json:"uuid" example:"3d5a…"`
	Name      string `json:"name" example:"report"`
	BotUUID   string `json:"bot_uuid" example:"bot-uuid"`
	ChatID    string `json:"chat_id" example:"123456789"`
	Exclusive bool   `json:"exclusive" example:"false"`
	Enabled   bool   `json:"enabled" example:"true"`
	Online    bool   `json:"online" example:"false"`
	CreatedAt string `json:"created_at" example:"2026-08-04T12:00:00Z"`
	UpdatedAt string `json:"updated_at" example:"2026-08-04T12:00:00Z"`
}

// CreateSubscriptionRequest is the body of POST /subscriptions.
type CreateSubscriptionRequest struct {
	Name      string `json:"name" binding:"required" example:"report"`
	BotUUID   string `json:"bot_uuid" binding:"required" example:"bot-uuid"`
	ChatID    string `json:"chat_id" binding:"required" example:"123456789"`
	Exclusive bool   `json:"exclusive,omitempty" example:"false"`
	Enabled   *bool  `json:"enabled,omitempty" example:"true"`
}

// CreateSubscriptionResponse carries the one-time plaintext token.
type CreateSubscriptionResponse struct {
	Subscription SubscriptionView `json:"subscription"`
	Token        string           `json:"token" example:"tb-sub-…"`
}

// UpdateSubscriptionRequest is the partial body of PUT /subscriptions/:id.
type UpdateSubscriptionRequest struct {
	Name      *string `json:"name,omitempty" example:"report"`
	BotUUID   *string `json:"bot_uuid,omitempty" example:"bot-uuid"`
	ChatID    *string `json:"chat_id,omitempty" example:"123456789"`
	Exclusive *bool   `json:"exclusive,omitempty" example:"true"`
	Enabled   *bool   `json:"enabled,omitempty" example:"false"`
}

// SubscriptionResponse wraps one subscription.
type SubscriptionResponse struct {
	Subscription SubscriptionView `json:"subscription"`
}

// SubscriptionListResponse wraps the list.
type SubscriptionListResponse struct {
	Subscriptions []SubscriptionView `json:"subscriptions"`
}

// RotateTokenResponse carries the one-time replacement token.
type RotateTokenResponse struct {
	Token string `json:"token" example:"tb-sub-…"`
}

// NotifyRequest is the body of POST /subscriptions/:id/notify. The target
// chat is the subscription's binding — no target field by design.
type NotifyRequest struct {
	Title string `json:"title,omitempty" example:"Build #412 failed"`
	Body  string `json:"body" binding:"required" example:"main branch is red"`
	Level string `json:"level,omitempty" example:"info"`
}

// InteractRequest is the body of POST /subscriptions/:id/interact.
type InteractRequest struct {
	Kind           string               `json:"kind" binding:"required" example:"confirm"`
	Title          string               `json:"title" binding:"required" example:"Deploy to prod?"`
	Body           string               `json:"body,omitempty" example:"commit a1b2c3"`
	Options        []interaction.Option `json:"options,omitempty"`
	TimeoutSeconds int                  `json:"timeout_seconds,omitempty" example:"120"`
}

// InteractResponse is the 202 body of POST /subscriptions/:id/interact.
type InteractResponse struct {
	RequestID string `json:"request_id" example:"a1b2…"`
	WaitURL   string `json:"wait_url" example:"/api/v1/subscriptions/{id}/interact/{request_id}"`
	ExpiresAt string `json:"expires_at" example:"2026-08-04T12:05:00Z"`
}

// EventView is one inbound mailbox event.
type EventView struct {
	ID        int64  `json:"id" example:"42"`
	ChatID    string `json:"chat_id" example:"123456789"`
	SenderID  string `json:"sender_id" example:"987654321"`
	MessageID string `json:"message_id,omitempty" example:"1024"`
	Text      string `json:"text" example:"run job 7"`
	CreatedAt string `json:"created_at" example:"2026-08-04T12:00:00Z"`
}

// EventsResponse is the body of GET /subscriptions/:id/events.
type EventsResponse struct {
	Events []EventView `json:"events"`
}

// AckRequest is the body of POST /subscriptions/:id/events/ack.
type AckRequest struct {
	UpTo int64 `json:"up_to" binding:"required" example:"42"`
}

// ReplyRequest is the body of POST /subscriptions/:id/reply.
type ReplyRequest struct {
	Text string `json:"text" binding:"required" example:"job 7 restarted ✅"`
	// EventID threads the reply to the inbound event's message when given.
	EventID int64 `json:"event_id,omitempty" example:"42"`
}

// OKResponse is the generic mutation result.
type OKResponse struct {
	OK bool `json:"ok" example:"true"`
}

// RegisterControlRoutes registers the CRUD + token surface on the operator
// control-plane group (the existing apiV1 group with UserAuth middleware).
func RegisterControlRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.GET("/subscriptions", handler.List,
		swagger.WithTags("subscription"),
		swagger.WithDescription("List subscriptions (no tokens)."),
		swagger.WithResponseModel(SubscriptionListResponse{}))

	router.POST("/subscriptions", handler.Create,
		swagger.WithTags("subscription"),
		swagger.WithDescription("Create a subscription; the scoped tb-sub- token is returned exactly once."),
		swagger.WithRequestModel(CreateSubscriptionRequest{}),
		swagger.WithResponseModel(CreateSubscriptionResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid name or missing fields"},
			swagger.ErrorResponseConfig{Code: 409, Message: "Name already taken"},
		))

	router.GET("/subscriptions/:id", handler.Get,
		swagger.WithTags("subscription"),
		swagger.WithDescription("Get one subscription."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithResponseModel(SubscriptionResponse{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: 404, Message: "Not found"}))

	router.PUT("/subscriptions/:id", handler.Update,
		swagger.WithTags("subscription"),
		swagger.WithDescription("Update name/bot/chat/exclusive/enabled (partial)."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithRequestModel(UpdateSubscriptionRequest{}),
		swagger.WithResponseModel(SubscriptionResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "Not found"},
			swagger.ErrorResponseConfig{Code: 409, Message: "Name already taken"},
		))

	router.DELETE("/subscriptions/:id", handler.Delete,
		swagger.WithTags("subscription"),
		swagger.WithDescription("Delete the subscription and its queued events."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithResponseModel(OKResponse{}))

	router.POST("/subscriptions/:id/token", handler.RotateToken,
		swagger.WithTags("subscription"),
		swagger.WithDescription("Rotate the scoped token; the old one stops working immediately."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithResponseModel(RotateTokenResponse{}),
		swagger.WithErrorResponses(swagger.ErrorResponseConfig{Code: 404, Message: "Not found"}))
}

// RegisterDataRoutes registers the tool-facing data plane on a group guarded
// by DataAuthMiddleware (subscription token or operator token).
func RegisterDataRoutes(router *swagger.RouteGroup, handler *Handler) {
	router.POST("/subscriptions/:id/notify", handler.Notify,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("One-way push into the subscription's bound chat, attributed with its name. Accepts the subscription's tb-sub- token."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithRequestModel(NotifyRequest{}),
		swagger.WithResponseModel(OKResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 401, Message: "Invalid token"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Subscription not available or bot not running"},
		))

	router.POST("/subscriptions/:id/interact", handler.Interact,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("Start an interactive prompt in the bound chat; long-poll the returned request_id for the reply."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithRequestModel(InteractRequest{}),
		swagger.WithResponseModel(InteractResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 401, Message: "Invalid token"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Subscription not available or bot not running"},
		))

	router.GET("/subscriptions/:id/interact/:request_id", handler.Wait,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("Long-poll the reply to an interactive prompt (200 answered/cancelled, 410 timeout, 504 pending, 404 expired)."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithPathParam("request_id", "string", "Interaction request id"))

	router.GET("/subscriptions/:id/events", handler.Events,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("Long-poll the inbound mailbox: events past the acked cursor, oldest first. Ack via /events/ack."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithResponseModel(EventsResponse{}))

	router.POST("/subscriptions/:id/events/ack", handler.AckEvents,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("Advance the mailbox cursor; acked events are pruned. The cursor never moves backwards."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithRequestModel(AckRequest{}),
		swagger.WithResponseModel(OKResponse{}))

	router.POST("/subscriptions/:id/reply", handler.Reply,
		swagger.WithTags("subscription-data"),
		swagger.WithDescription("Send into the bound chat; threads to the referenced inbound event when event_id is given."),
		swagger.WithPathParam("id", "string", "Subscription UUID"),
		swagger.WithRequestModel(ReplyRequest{}),
		swagger.WithResponseModel(OKResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 401, Message: "Invalid token"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Subscription not available or bot not running"},
		))
}
