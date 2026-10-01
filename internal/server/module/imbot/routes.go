package imbot

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// RegisterRoutes registers all ImBot settings routes with swagger documentation
var _ module.Module = (*Handler)(nil)

func (h *Handler) RegisterRoutes(rt *module.Routes) {
	router := rt.V1
	registerAccessRoutes(router, h)
	// GET /imbot-settings - List all ImBot configurations
	router.GET("/imbot-settings", h.ListSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all ImBot configurations"),
		// Explicit name: "ListResponse" collides with team.ListResponse in
		// the shared OpenAPI schema namespace (both packages had a bare
		// ListResponse type) — without this, whichever gets generated last
		// silently overwrites the other's shape in openapi.json.
		swagger.WithResponseModel(ListResponse{}, "ImBotSettingsListResponse"),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "ImBot settings store not available"},
		),
	)

	// GET /imbot-settings/:uuid - Get a single ImBot configuration
	router.GET("/imbot-settings/:uuid", h.GetSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns a single ImBot configuration by UUID"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-settings - Create a new ImBot configuration
	router.POST("/imbot-settings", h.CreateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Creates a new ImBot configuration"),
		// Explicit name: bare "CreateRequest" collides with team.CreateRequest
		// — see the ListResponse comment above.
		swagger.WithRequestModel(CreateRequest{}, "ImBotSettingsCreateRequest"),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
		),
	)

	// PUT /imbot-settings/:uuid - Update an existing ImBot configuration
	router.PUT("/imbot-settings/:uuid", h.UpdateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Updates an existing ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		// Explicit name: bare "UpdateRequest" collides with team.UpdateRequest
		// — see the ListResponse comment above.
		swagger.WithRequestModel(UpdateRequest{}, "ImBotSettingsUpdateRequest"),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// DELETE /imbot-settings/:uuid - Delete an ImBot configuration
	router.DELETE("/imbot-settings/:uuid", h.DeleteSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Deletes an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(DeleteResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-settings/:uuid/toggle - Toggle enabled status
	router.POST("/imbot-settings/:uuid/toggle", h.ToggleSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Toggles the enabled status of an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(ToggleResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// GET /imbot-settings/:uuid/pairing-code - Reveal current TOFU pairing code
	router.GET("/imbot-settings/:uuid/pairing-code", h.GetPairingCode,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Reveals the bot's current TOFU pairing code; every reveal is audit-logged"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(PairingCodeResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-settings/:uuid/pairing-code/rotate - Mint a fresh pairing code
	router.POST("/imbot-settings/:uuid/pairing-code/rotate", h.RotatePairingCode,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Mints a new TOFU pairing code, invalidating the previous one"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(PairingCodeResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "TOFU pairing not enabled for this bot"},
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-admin/restart/:uuid - Restart a single bot in place
	router.POST("/imbot-admin/restart/:uuid", h.RestartBot,
		swagger.WithTags("imbot-admin"),
		swagger.WithDescription("Stops and restarts a single bot without affecting other bots or the HTTP server"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "UUID is required"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Bot manager not available"},
		),
	)

	// POST /imbot-admin/reload - Reload bot configurations and reconcile enabled state
	router.POST("/imbot-admin/reload", h.Reload,
		swagger.WithTags("imbot-admin"),
		swagger.WithDescription("Re-reads bot settings and starts/stops bots to match enabled flags"),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Bot manager not available"},
		),
	)

	// GET /imbot-platforms - Get all supported platforms
	router.GET("/imbot-platforms", h.GetPlatforms,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all supported ImBot platforms with their configurations"),
		swagger.WithResponseModel(PlatformsResponse{}),
	)

	// GET /imbot-platform-config - Get platform auth configuration
	router.GET("/imbot-platform-config", h.GetPlatformConfig,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns auth configuration for a specific platform"),
		swagger.WithQueryConfig("platform", swagger.QueryParamConfig{
			Name:        "platform",
			Type:        "string",
			Required:    true,
			Description: "Platform identifier (telegram, discord, slack, feishu, dingtalk, whatsapp, weixin)",
		}),
		swagger.WithResponseModel(PlatformConfigResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Platform parameter is required"},
			swagger.ErrorResponseConfig{Code: 404, Message: "Unknown platform"},
		),
	)

	// Feishu/Lark one-click registration endpoints (OAuth 2.0 Device Authorization Grant)
	if fr := h.feishuRegHandler; fr != nil {
		// POST /imbot-settings/:uuid/feishu/qr-start - Start one-click app registration
		router.POST("/imbot-settings/:uuid/feishu/qr-start", fr.QRStart,
			swagger.WithTags("imbot-settings", "feishu"),
			swagger.WithDescription("Starts Feishu/Lark one-click app registration and returns a QR verification link"),
			swagger.WithPathParam("uuid", "string", "ImBot configuration UUID (use a temp- prefix for deferred creation)"),
			swagger.WithRequestModel(FeishuRegStartRequest{}),
			swagger.WithResponseModel(FeishuRegStartResponse{}),
		)

		// GET /imbot-settings/:uuid/feishu/qr-status - Poll one-click registration status
		router.GET("/imbot-settings/:uuid/feishu/qr-status", fr.QRStatus,
			swagger.WithTags("imbot-settings", "feishu"),
			swagger.WithDescription("Polls Feishu/Lark one-click app registration status"),
			swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
			swagger.WithResponseModel(FeishuRegStatusResponse{}),
			swagger.WithErrorResponses(
				swagger.ErrorResponseConfig{Code: 404, Message: "No active registration session found"},
			),
		)

		// POST /imbot-settings/:uuid/feishu/qr-cancel - Cancel pending registration
		router.POST("/imbot-settings/:uuid/feishu/qr-cancel", fr.QRCancel,
			swagger.WithTags("imbot-settings", "feishu"),
			swagger.WithDescription("Cancels a pending Feishu/Lark one-click app registration"),
			swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		)
	}

	// Weixin QR Login endpoints - use h's persistent QR login h
	qrHandler := h.qrLoginHandler
	if qrHandler == nil {
		logrus.Warn("WeChat QR login h is nil, QR login endpoints will not be available")
		return
	}

	// POST /imbot-settings/:uuid/weixin/qr-start - Start QR login
	router.POST("/imbot-settings/:uuid/weixin/qr-start", qrHandler.QRStart,
		swagger.WithTags("imbot-settings", "weixin"),
		swagger.WithDescription("Initiates Weixin QR code login flow"),
		swagger.WithRequestModel(QRStartRequest{}),
		swagger.WithResponseModel(QRStartResponse{}),
	)

	// GET /imbot-settings/:uuid/weixin/qr-status - Poll QR login status
	router.GET("/imbot-settings/:uuid/weixin/qr-status", qrHandler.QRStatus,
		swagger.WithTags("imbot-settings", "weixin"),
		swagger.WithDescription("Polls Weixin QR code login status"),
		swagger.WithQueryRequired("qrcode_id", "string", "QR code session identifier"),
		swagger.WithResponseModel(QRStatusResponse{}),
	)

	// POST /imbot-settings/:uuid/weixin/qr-cancel - Cancel QR login
	router.POST("/imbot-settings/:uuid/weixin/qr-cancel", qrHandler.QRCancel,
		swagger.WithTags("imbot-settings", "weixin"),
		swagger.WithDescription("Cancels pending Weixin QR code login"),
	)
}
