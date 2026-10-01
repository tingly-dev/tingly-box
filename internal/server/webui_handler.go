package server

import (
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/pkg/auth"
)

// WebDeps declares exactly what WebHandler needs from the host server. It is
// populated once, from server.NewServer, after all of *Server's fields have
// been constructed.
type WebDeps struct {
	// MemoryLogMW backs the HTTP request log API (GetLogs/GetLogStats/ClearLogs).
	MemoryLogMW *middleware.MemoryLog

	// MultiLogger backs the system log, model-request trace and action
	// history APIs.
	MultiLogger *obs.MultiLogger

	// Config backs token generation/retrieval (model token persistence).
	Config *config.Config

	// JWTManager issues the JWT-backed model tokens.
	JWTManager *auth.JWTManager
}

// WebHandler serves the WebUI Management API's server-control surface:
// status/start/restart (status_handler.go), HTTP/system/action logs
// (log_handler.go), correlated model requests (model_request_handler.go) and
// model-token management (token_handler.go).
type WebHandler struct {
	deps WebDeps
}

// NewWebHandler constructs the WebUI control handler from its dependencies.
func NewWebHandler(deps WebDeps) *WebHandler {
	return &WebHandler{deps: deps}
}
