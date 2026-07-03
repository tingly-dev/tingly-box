package server

import (
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/pkg/auth"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// WebUIDeps declares exactly what the WebUI Management API's control
// handlers need from the host server. It is populated and passed in once,
// from server.NewServer, after all of *Server's fields have been constructed.
//
// This grows as each subsequent migration step moves a file in
// (server_control.go, guardrails_handler.go, etc.) and wires up the
// fields/methods it actually touches on *Server today.
type WebUIDeps struct {
	// MemoryLogMW backs the HTTP request log API (GetLogs/GetLogStats/ClearLogs).
	MemoryLogMW *middleware.MultiModeMemoryLogMiddleware

	// MultiLogger backs the system log, model-request trace and action
	// history APIs.
	MultiLogger *obs.MultiLogger

	// Config backs token generation/retrieval (model token persistence).
	Config *config.Config

	// JWTManager issues the JWT-backed model tokens.
	JWTManager *auth.JWTManager
}

// WebUIHandler is the aggregate handler for the WebUI Management API's
// server-control surface (status/start/stop, logs, guardrails admin, token
// management, etc). Individual method files will be moved here in later
// steps and become methods on *WebUIHandler.
type WebUIHandler struct {
	deps WebUIDeps
}

// NewControlHandler constructs the WebUI control handler from its dependencies.
func NewControlHandler(deps WebUIDeps) *WebUIHandler {
	return &WebUIHandler{deps: deps}
}
