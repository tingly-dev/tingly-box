package imbot

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers all ImBot settings routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, handler *Handler) {
	// GET /imbot-settings - List all ImBot configurations
	router.GET("/imbot-settings", handler.ListSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all ImBot configurations"),
		swagger.WithResponseModel(ListResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "ImBot settings store not available"},
		),
	)

	// GET /imbot-settings/:uuid - Get a single ImBot configuration
	router.GET("/imbot-settings/:uuid", handler.GetSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns a single ImBot configuration by UUID"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-settings - Create a new ImBot configuration
	router.POST("/imbot-settings", handler.CreateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Creates a new ImBot configuration"),
		swagger.WithRequestModel(CreateRequest{}),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
		),
	)

	// PUT /imbot-settings/:uuid - Update an existing ImBot configuration
	router.PUT("/imbot-settings/:uuid", handler.UpdateSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Updates an existing ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithRequestModel(UpdateRequest{}),
		swagger.WithResponseModel(SettingsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// DELETE /imbot-settings/:uuid - Delete an ImBot configuration
	router.DELETE("/imbot-settings/:uuid", handler.DeleteSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Deletes an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(DeleteResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// POST /imbot-settings/:uuid/toggle - Toggle enabled status
	router.POST("/imbot-settings/:uuid/toggle", handler.ToggleSettings,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Toggles the enabled status of an ImBot configuration"),
		swagger.WithPathParam("uuid", "string", "ImBot configuration UUID"),
		swagger.WithResponseModel(ToggleResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 404, Message: "ImBot settings not found"},
		),
	)

	// GET /imbot-platforms - Get all supported platforms
	router.GET("/imbot-platforms", handler.GetPlatforms,
		swagger.WithTags("imbot-settings"),
		swagger.WithDescription("Returns all supported ImBot platforms with their configurations"),
		swagger.WithResponseModel(PlatformsResponse{}),
	)

	// GET /imbot-platform-config - Get platform auth configuration
	router.GET("/imbot-platform-config", handler.GetPlatformConfig,
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

	// Weixin QR Login endpoints - use handler's persistent QR login handler
	qrHandler := handler.qrLoginHandler
	if qrHandler == nil {
		logrus.Warn("WeChat QR login handler is nil, QR login endpoints will not be available")
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
		swagger.WithResponseModel(QRStatusResponse{}),
	)

	// POST /imbot-settings/:uuid/weixin/qr-cancel - Cancel QR login
	router.POST("/imbot-settings/:uuid/weixin/qr-cancel", qrHandler.QRCancel,
		swagger.WithTags("imbot-settings", "weixin"),
		swagger.WithDescription("Cancels pending Weixin QR code login"),
	)
}
