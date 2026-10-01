package notify

import (
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/swagger"
)

// BotNotifyRequest is the swagger model for POST /bots/:bot/notify.
type BotNotifyRequest struct {
	Target access.TargetRef `json:"target"`
	Title  string           `json:"title,omitempty" example:"Build #412 failed"`
	Body   string           `json:"body" example:"main branch is red"`
	Level  string           `json:"level,omitempty" example:"info"`
}

// BotInteractRequest is the swagger model for POST /bots/:bot/interact.
type BotInteractRequest struct {
	Target         access.TargetRef    `json:"target"`
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
	Disabled      bool   `json:"disabled,omitempty" example:"false"`
	DisabledAt    string `json:"disabled_at,omitempty" example:"2026-07-28T12:00:00Z"`
	UpdatedAt     string `json:"updated_at,omitempty" example:"2026-07-25T12:00:00Z"`
}

// BotChatSetDisabledRequest is the swagger model for
// PUT /bots/:bot/chats/:chat_id/disabled.
type BotChatSetDisabledRequest struct {
	Disabled *bool `json:"disabled" example:"true"`
}

// BotChatOKResponse is the swagger model for the chat mutation endpoints.
type BotChatOKResponse struct {
	OK bool `json:"ok" example:"true"`
}

// BotNotifyResponse is the swagger model for the POST /bots/:bot/notify
// success body (200). Mirrors bot_api.go's Notify handler literally
// (gin.H{"ok": true}) — kept in sync by hand since the handler builds the
// map directly rather than marshaling this struct.
type BotNotifyResponse struct {
	OK bool `json:"ok" example:"true"`
}

// BotInteractStartResponse is the swagger model for the POST
// /bots/:bot/interact success body (202). Mirrors bot_api.go's Interact
// handler literally (gin.H{"request_id", "wait_url", "expires_at"}).
type BotInteractStartResponse struct {
	RequestID string `json:"request_id" example:"a1b2c3d4"`
	WaitURL   string `json:"wait_url" example:"/api/v1/bots/abc/interact/a1b2c3d4"`
	ExpiresAt string `json:"expires_at" example:"2026-07-25T12:02:00Z"`
}

// BotInteractWaitResponse is the swagger model for the GET
// /bots/:bot/interact/:request_id body — returned on both 200 (answered /
// cancelled) and 410 (timeout / error); Reason is only set in the latter
// case. Mirrors handler.go's respondResult literally.
type BotInteractWaitResponse struct {
	Status   string         `json:"status" example:"answered"`
	Decision map[string]any `json:"decision,omitempty"`
	Reason   string         `json:"reason,omitempty" example:"no reply within timeout"`
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
// The group's base path determines the full URL; registered under the V1 group this
// yields /api/v1/bots/:bot/... — see .design/bot-interaction-api.md.
var _ module.Module = (*BotAPIHandler)(nil)

func (h *BotAPIHandler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	router.POST("/bots/:bot/notify", h.Notify,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Deliver a one-way notification to a running bot's chat. Requires the operator user token."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithRequestModel(BotNotifyRequest{}),
		swagger.WithResponseModel(BotNotifyResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request body"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Bot not running"},
			swagger.ErrorResponseConfig{Code: 500, Message: "Delivery failed"},
		),
	)

	router.POST("/bots/:bot/interact", h.Interact,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Start an interactive prompt on a running bot's chat and return a request_id to long-poll for the reply."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithRequestModel(BotInteractRequest{}),
		swagger.WithResponseModel(BotInteractStartResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request body or kind"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Bot not running"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Interaction registry unavailable"},
		),
	)

	router.GET("/bots/:bot/interact/:request_id", h.Wait,
		swagger.WithTags("bot-interaction"),
		swagger.WithDescription("Long-poll for the reply to an interactive prompt started by POST /bots/:bot/interact."),
		swagger.WithPathParam("bot", "string", "Target bot UUID"),
		swagger.WithPathParam("request_id", "string", "Interaction request id from the interact response"),
		swagger.WithQuery("timeout", "string", "Long-poll budget, e.g. 45s (default 45s, capped at 50s)"),
		swagger.WithResponseModel(BotInteractWaitResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "Request expired"},
			swagger.ErrorResponseConfig{Code: 410, Message: "Prompt timed out or ended in error"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Interaction registry unavailable"},
			swagger.ErrorResponseConfig{Code: 504, Message: "No reply yet — retry (long-poll pending)"},
		),
	)

}
