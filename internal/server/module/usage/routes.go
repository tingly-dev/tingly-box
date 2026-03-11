package usage

import (
	"github.com/tingly-dev/tingly-box/pkg/swagger"
)

// RegisterRoutes registers the usage API routes with swagger documentation
func RegisterRoutes(router *swagger.RouteGroup, api *API) {
	// GET /api/v1/usage/stats - Get aggregated usage statistics
	router.GET("/usage/stats", api.GetStats,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns aggregated usage statistics with flexible grouping and filtering"),
		swagger.WithQueryConfig("group_by", swagger.QueryParamConfig{
			Name:        "group_by",
			Type:        "string",
			Required:    false,
			Description: "Aggregation level: model, provider, scenario, rule, user, daily, hourly",
			Default:     "model",
			Enum:        []interface{}{"model", "provider", "scenario", "rule", "user", "daily", "hourly"},
		}),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("rule_uuid", swagger.QueryParamConfig{
			Name:        "rule_uuid",
			Type:        "string",
			Required:    false,
			Description: "Filter by rule UUID",
		}),
		swagger.WithQueryConfig("user_id", swagger.QueryParamConfig{
			Name:        "user_id",
			Type:        "string",
			Required:    false,
			Description: "Filter by enterprise user ID",
		}),
		swagger.WithQueryConfig("status", swagger.QueryParamConfig{
			Name:        "status",
			Type:        "string",
			Required:    false,
			Description: "Filter by status: success, error, partial",
			Default:     "success",
			Enum:        []interface{}{"success", "error", "partial"},
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Max results to return",
			Default:     100,
			Minimum:     intPtr(1),
			Maximum:     intPtr(1000),
		}),
		swagger.WithQueryConfig("sort_by", swagger.QueryParamConfig{
			Name:        "sort_by",
			Type:        "string",
			Required:    false,
			Description: "Sort field: total_tokens, request_count, avg_latency",
			Default:     "total_tokens",
			Enum:        []interface{}{"total_tokens", "request_count", "avg_latency"},
		}),
		swagger.WithQueryConfig("sort_order", swagger.QueryParamConfig{
			Name:        "sort_order",
			Type:        "string",
			Required:    false,
			Description: "asc or desc",
			Default:     "desc",
			Enum:        []interface{}{"asc", "desc"},
		}),
		swagger.WithResponseModel(UsageStatsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// GET /api/v1/usage/timeseries - Get time-series usage data
	router.GET("/usage/timeseries", api.GetTimeSeries,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns time-series data for usage with configurable intervals"),
		swagger.WithQueryConfig("interval", swagger.QueryParamConfig{
			Name:        "interval",
			Type:        "string",
			Required:    false,
			Description: "Time bucket: minute, hour, day, week",
			Default:     "hour",
			Enum:        []interface{}{"minute", "hour", "day", "week"},
		}),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithResponseModel(TimeSeriesResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// GET /api/v1/usage/records - Get individual usage records
	router.GET("/usage/records", api.GetRecords,
		swagger.WithTags("usage"),
		swagger.WithDescription("Returns individual usage records (for debugging/audit)"),
		swagger.WithQueryConfig("start_time", swagger.QueryParamConfig{
			Name:        "start_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 start time",
		}),
		swagger.WithQueryConfig("end_time", swagger.QueryParamConfig{
			Name:        "end_time",
			Type:        "string",
			Required:    false,
			Description: "ISO 8601 end time",
		}),
		swagger.WithQueryConfig("provider", swagger.QueryParamConfig{
			Name:        "provider",
			Type:        "string",
			Required:    false,
			Description: "Filter by provider UUID",
		}),
		swagger.WithQueryConfig("model", swagger.QueryParamConfig{
			Name:        "model",
			Type:        "string",
			Required:    false,
			Description: "Filter by model name",
		}),
		swagger.WithQueryConfig("scenario", swagger.QueryParamConfig{
			Name:        "scenario",
			Type:        "string",
			Required:    false,
			Description: "Filter by scenario",
		}),
		swagger.WithQueryConfig("status", swagger.QueryParamConfig{
			Name:        "status",
			Type:        "string",
			Required:    false,
			Description: "Filter by status",
		}),
		swagger.WithQueryConfig("limit", swagger.QueryParamConfig{
			Name:        "limit",
			Type:        "integer",
			Required:    false,
			Description: "Max results (max 1000)",
			Default:     50,
			Minimum:     intPtr(1),
			Maximum:     intPtr(1000),
		}),
		swagger.WithQueryConfig("offset", swagger.QueryParamConfig{
			Name:        "offset",
			Type:        "integer",
			Required:    false,
			Description: "Pagination offset",
			Default:     0,
			Minimum:     intPtr(0),
		}),
		swagger.WithResponseModel(UsageRecordsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)

	// DELETE /api/v1/usage/records - Delete old usage records
	router.DELETE("/usage/records", api.DeleteOldRecords,
		swagger.WithTags("usage"),
		swagger.WithDescription("Deletes usage records older than the specified number of days"),
		swagger.WithRequestModel(DeleteOldRecordsRequest{}),
		swagger.WithResponseModel(DeleteOldRecordsResponse{}),
		swagger.WithErrorResponses(
			swagger.ErrorResponseConfig{Code: 400, Message: "Invalid request"},
			swagger.ErrorResponseConfig{Code: 503, Message: "Usage store not available"},
		),
	)
}
