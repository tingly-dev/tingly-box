package settings

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/llm"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// SettingsResponse is the connection as shown to the UI. The API key itself
// never leaves the server; only whether one is set and its last characters.
type SettingsResponse struct {
	APIStyle       string `json:"api_style" example:"anthropic"`
	BaseURL        string `json:"base_url" example:"http://localhost:12580/tingly/anthropic"`
	Endpoint       string `json:"endpoint" example:"http://localhost:12580/tingly/anthropic/v1/messages"`
	APIKeySet      bool   `json:"api_key_set"`
	APIKeyHint     string `json:"api_key_hint" example:"…a1b2"`
	DefaultModel   string `json:"default_model"`
	DefaultBaseURL string `json:"default_base_url"`
	// Threads describes how project threads (Claude Code sessions) run.
	Threads ThreadRuntime `json:"threads"`
}

// ThreadRuntime is the Claude Code CLI threads run with.
type ThreadRuntime struct {
	CLIPath string `json:"cli_path"`
	// ViaConnection is true when threads use this connection (Anthropic
	// style); otherwise they use the CLI's own login.
	ViaConnection bool `json:"via_connection"`
}

// ConnectionDraft is a connection as edited in the settings form. A nil
// APIKey keeps the saved key; an empty string clears it.
type ConnectionDraft struct {
	APIStyle     string  `json:"api_style" binding:"required,oneof=anthropic openai"`
	BaseURL      string  `json:"base_url" binding:"required"`
	APIKey       *string `json:"api_key,omitempty"`
	DefaultModel string  `json:"default_model"`
}

// ModelsResponse lists models offered by an endpoint.
type ModelsResponse struct {
	Models []llm.Model `json:"models"`
	// Error is set when listing failed; the UI then offers free-text model entry.
	Error string `json:"error,omitempty"`
}

// TestRequest checks a draft connection with a real chat request.
type TestRequest struct {
	ConnectionDraft
	Model string `json:"model,omitempty"`
}

// TestResponse reports what the test request did and got back.
type TestResponse struct {
	OK        bool   `json:"ok"`
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	LatencyMS int64  `json:"latency_ms"`
	Reply     string `json:"reply,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Handler is the settings HTTP module.
type Handler struct {
	svc *Service
}

// NewHandler builds the settings module.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts the settings endpoints.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	api := rt.API
	api.GET("/settings", h.get,
		swagger.WithTags("settings"),
		swagger.WithDescription("Get the model connection settings"),
		swagger.WithResponseModel(SettingsResponse{}))
	api.PUT("/settings", h.put,
		swagger.WithTags("settings"),
		swagger.WithDescription("Save the model connection settings"),
		swagger.WithRequestModel(ConnectionDraft{}),
		swagger.WithResponseModel(SettingsResponse{}))
	api.POST("/settings/test", h.test,
		swagger.WithTags("settings"),
		swagger.WithDescription("Send a real chat request through a draft connection"),
		swagger.WithRequestModel(TestRequest{}),
		swagger.WithResponseModel(TestResponse{}))
	api.POST("/settings/models", h.draftModels,
		swagger.WithTags("settings"),
		swagger.WithDescription("List models offered by a draft connection"),
		swagger.WithRequestModel(ConnectionDraft{}),
		swagger.WithResponseModel(ModelsResponse{}))
	api.GET("/models", h.models,
		swagger.WithTags("settings"),
		swagger.WithDescription("List models offered by the saved connection"),
		swagger.WithResponseModel(ModelsResponse{}))
}

func toResponse(conn Connection) SettingsResponse {
	hint := ""
	if n := len(conn.APIKey); n > 0 {
		if n > 8 {
			hint = "…" + conn.APIKey[n-4:]
		} else {
			hint = "…"
		}
	}
	return SettingsResponse{
		APIStyle:       conn.APIStyle,
		BaseURL:        conn.BaseURL,
		Endpoint:       llm.Endpoint(conn.LLM()),
		APIKeySet:      conn.APIKey != "",
		APIKeyHint:     hint,
		DefaultModel:   conn.DefaultModel,
		DefaultBaseURL: DefaultBaseURL,
		Threads:        ThreadRuntime{CLIPath: project.ClaudeCLIPath(), ViaConnection: conn.APIStyle == llm.StyleAnthropic},
	}
}

// apply overlays a draft on the saved connection.
func (h *Handler) apply(d ConnectionDraft) (Connection, error) {
	saved, err := h.svc.Connection()
	if err != nil {
		return saved, err
	}
	conn := Connection{
		APIStyle:     d.APIStyle,
		BaseURL:      strings.TrimSpace(d.BaseURL),
		APIKey:       saved.APIKey,
		DefaultModel: strings.TrimSpace(d.DefaultModel),
	}
	if d.APIKey != nil {
		conn.APIKey = strings.TrimSpace(*d.APIKey)
	}
	return conn, nil
}

func (h *Handler) get(c *gin.Context) {
	conn, err := h.svc.Connection()
	if err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	c.JSON(200, toResponse(conn))
}

func (h *Handler) put(c *gin.Context) {
	var d ConnectionDraft
	if !apierr.Bind(c, &d) {
		return
	}
	conn, err := h.apply(d)
	if err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	if err := h.svc.Save(conn); err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	c.JSON(200, toResponse(conn))
}

func (h *Handler) models(c *gin.Context) {
	conn, err := h.svc.Connection()
	if err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	models, err := h.svc.ModelsFor(c.Request.Context(), conn)
	resp := ModelsResponse{Models: models}
	if resp.Models == nil {
		resp.Models = []llm.Model{}
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(200, resp)
}

func (h *Handler) draftModels(c *gin.Context) {
	var d ConnectionDraft
	if !apierr.Bind(c, &d) {
		return
	}
	conn, err := h.apply(d)
	if err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	models, err := h.svc.ModelsFor(c.Request.Context(), conn)
	resp := ModelsResponse{Models: models}
	if resp.Models == nil {
		resp.Models = []llm.Model{}
	}
	if err != nil {
		resp.Error = err.Error()
	}
	c.JSON(200, resp)
}

func (h *Handler) test(c *gin.Context) {
	var req TestRequest
	if !apierr.Bind(c, &req) {
		return
	}
	conn, err := h.apply(req.ConnectionDraft)
	if err != nil {
		apierr.Store(c, err, "settings")
		return
	}
	resp := TestResponse{Endpoint: llm.Endpoint(conn.LLM())}
	model, err := h.svc.ResolveModel(c.Request.Context(), conn, req.Model)
	if err != nil {
		resp.Error = err.Error()
		c.JSON(200, resp)
		return
	}
	resp.Model = model
	client, err := llm.New(conn.LLM())
	if err != nil {
		resp.Error = err.Error()
		c.JSON(200, resp)
		return
	}
	start := time.Now()
	res, err := llm.Complete(c.Request.Context(), client, llm.Request{
		Model:     model,
		Messages:  []llm.Message{{Role: "user", Content: "Reply with exactly: pong"}},
		MaxTokens: 16,
	})
	resp.LatencyMS = time.Since(start).Milliseconds()
	resp.Reply = strings.TrimSpace(res.Text)
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.OK = true
	}
	c.JSON(200, resp)
}
