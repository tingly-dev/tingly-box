package notify

import (
	"github.com/tingly-dev/tingly-box/swagger"
)

// BotNotifyRequest is the swagger model for POST /bots/:bot/notify.
type BotNotifyRequest struct {
	ChatID string `json:"chat_id" example:"dm:ops"`
	Title  string `json:"title,omitempty" example:"Build #412 failed"`
	Body   string `json:"body" example:"main branch is red"`
	Level  string `json:"level,omitempty" example:"info"`
}

// BotInteractRequest is the swagger model for POST /bots/:bot/interact.
type BotInteractRequest struct {
	ChatID         string              `json:"chat_id" example:"dm:ops"`
	Kind           string              `json:"kind" example:"confirm"`
	Title          string              `json:"title" example:"Deploy to prod?"`
	Body           string              `json:"body,omitempty" example:"commit a1b2c3"`
	Options        []BotInteractOption `json:"options,omitempty"`
	TimeoutSeconds int                 `json:"timeout_seconds,omitempty" example:"120"`
}

// BotInteractOption mirrors interaction.Option for the swagger surface.
type BotInteractOption struct {
	Value string `json:"value" example:"yes"`
	Label string `json:"label" example:"Yes"`
	Style string `json:"style,omitempty" example:"primary"`
}

// BotChatSummary is the swagger model for one entry in GET /bots/:bot/chats.
type BotChatSummary struct {
	ChatID        string `json:"chat_id" example:"telegram:123456789"`
	Platform      string `json:"platform,omitempty" example:"telegram"`
	IsPaired      bool   `json:"is_paired,omitempty" example:"true"`
	IsWhitelisted bool   `json:"is_whitelisted,omitempty" example:"false"`
	ProjectPath   string `json:"project_path,omitempty" example:"/home/user/proj"`
	UpdatedAt     string `json:"updated_at,omitempty" example:"2026-07-25T12:00:00Z"`
}

// BotChatsResponse is the swagger model for the GET /bots/:bot/chats body.
// Running is false when the bot's channel isn't registered — the list is then
// empty by definition, and the caller can say "start the bot" rather than
// "no chats yet".
type BotChatsResponse struct {
	Chats   []BotChatSummary `json:"chats"`
	Running bool             `json:"running" example:"true"`
}

// RegisterBotRoutes registers the general bot interaction API on a control-
// plane route group (the existing apiV1 group, which already applies
// getUserAuthMiddleware). Routes:
//
//	POST /bots/:bot/notify        one-way push
//	POST /bots/:bot/interact      start interactive
//	GET  /bots/:bot/interact/:id  long-poll for the reply
//	GET  /bots/:bot/chats         discover the chat_id the other three need
//
// The group's base path determines the full URL; registered under apiV1 this
// yields /api/v1/bots/:bot/... — see .design/bot-interaction-api.md.
func RegisterBotRoutes(router *swagger.RouteGroup, handler *BotAPIHandler) {
	router.POST("/bots/:bot/notify", handler.Notify,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Deliver a one-way notification to a running bot's chat. Requires the operator user token."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithRequestModel(BotNotifyRequest{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request body"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Bot not running"},
			swagger.ErrorResponseConfig{Code: 500, Message: "Delivery failed"},
		),
	)

	router.POST("/bots/:bot/interact", handler.Interact,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Start an interactive prompt on a running bot's chat and return a request_id to long-poll for the reply."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithRequestModel(BotInteractRequest{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request body or kind"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Bot not running"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Interaction registry unavailable"},
		),
	)

	router.GET("/bots/:bot/interact/:request_id", handler.Wait,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Long-poll for the reply to an interactive prompt started by POST /bots/:bot/interact."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithPathParam("request_id", "string", "Interaction request id from the interact response"),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "Request expired"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Interaction registry unavailable"},
		),
	)

	router.GET("/bots/:bot/chats", handler.ListChats,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("List the chats a running bot can reach. Use the returned chat_id as the chat_id field in POST /bots/:bot/notify and /interact. A bot that isn't running returns an empty list with running:false rather than an error."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithResponseModel(BotChatsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Chat listing unavailable"},
		),
	)
}
