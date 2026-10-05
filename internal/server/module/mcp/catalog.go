package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type validationError struct{ message string }

func (e *validationError) Error() string       { return e.message }
func invalid(format string, args ...any) error { return &validationError{fmt.Sprintf(format, args...)} }

var validID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
var errMissing = errors.New("MCP entry not found")

func validateConfig(cfg *typ.MCPRuntimeConfig) error {
	if cfg.RequestTimeout < 0 || cfg.RequestTimeout > 600 {
		return invalid("request_timeout must be between 0 and 600 seconds")
	}
	raw, _ := json.Marshal(cfg)
	var expanded typ.MCPRuntimeConfig
	_ = json.Unmarshal(raw, &expanded)
	mcpruntime.ExpandMCPRuntimeEnvRefs(&expanded)
	seen := map[string]bool{}
	for i := range cfg.Sources {
		s := &cfg.Sources[i]
		if s.Origin != "" && s.Origin != "builtin" && s.Origin != "external" {
			return invalid("unsupported source origin: %s", s.Origin)
		}
		if !validID.MatchString(s.ID) || strings.Contains(s.ID, "__") || s.ID == "builtin" {
			return invalid("invalid source ID: %s", s.ID)
		}
		if seen[s.ID] {
			return invalid("duplicate source ID: %s", s.ID)
		}
		seen[s.ID] = true
		if s.Transport == "" {
			if s.Advisor != nil {
				s.Transport = "advisor"
			} else {
				s.Transport = "stdio"
			}
		}
		if s.Visibility != "" && s.Visibility != typ.ToolVisibilityClient && s.Visibility != typ.ToolVisibilityServer {
			return invalid("invalid visibility for %s", s.ID)
		}
		switch s.Transport {
		case "stdio":
			if typ.IsMCPSourceEnabled(*s) && strings.TrimSpace(s.Command) == "" {
				return invalid("source %s requires a command", s.ID)
			}
		case "http", "sse":
			if s.Endpoint != "" {
				u, err := url.Parse(expanded.Sources[i].Endpoint)
				if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
					return invalid("source %s requires an HTTP(S) endpoint", s.ID)
				}
			} else if typ.IsMCPSourceEnabled(*s) {
				return invalid("source %s requires an endpoint", s.ID)
			}
		case "advisor":
			if s.ID != "advisor" {
				return invalid("advisor transport is reserved for the advisor source")
			}
		default:
			return invalid("unsupported transport: %s", s.Transport)
		}
		if s.ProxyURL != "" {
			u, err := url.Parse(expanded.Sources[i].ProxyURL)
			if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
				return invalid("invalid proxy URL for %s", s.ID)
			}
		}
	}
	if issues := mcpruntime.ValidateEnabledMCPSourceEnvRefs(cfg.Sources); len(issues) > 0 {
		return invalid("source %s: missing environment variable %s for %s", issues[0].SourceID, issues[0].VarName, issues[0].FieldPath)
	}
	seen = map[string]bool{}
	for i := range cfg.ClientProfiles {
		p := &cfg.ClientProfiles[i]
		if p.Sources == nil {
			p.Sources = []string{}
		}
		if p.Tools == nil {
			p.Tools = []string{}
		}
		if !validID.MatchString(p.ID) || strings.Contains(p.ID, "__") {
			return invalid("invalid client profile ID: %s", p.ID)
		}
		if seen[p.ID] {
			return invalid("duplicate client profile ID: %s", p.ID)
		}
		seen[p.ID] = true
		for _, id := range p.Sources {
			if id != "*" {
				found := false
				for _, s := range cfg.Sources {
					if s.ID == id {
						found = true
					}
				}
				if !found {
					return invalid("client %s references unknown source %s", p.ID, id)
				}
			}
		}
		for _, name := range p.Tools {
			if name != "*" {
				if _, _, ok := mcpruntime.ParseNormalizedToolName(name); !ok {
					return invalid("client %s requires normalized tool names", p.ID)
				}
			}
		}
	}
	return nil
}

func (h *Handler) configChanged(c *gin.Context) {
	if h.runtime != nil {
		h.runtime.Reconcile(c.Request.Context())
	}
	if h.transportHandler != nil {
		h.transportHandler.ResetAll()
	}
}

func (h *Handler) mutate(c *gin.Context, change func(*typ.MCPRuntimeConfig) error, status int) {
	if h.cfg == nil {
		c.JSON(http.StatusServiceUnavailable, MCPRuntimeConfigResponse{Error: "Global config unavailable"})
		return
	}
	next, err := h.cfg.UpdateMCPRuntimeConfig(func(cfg *typ.MCPRuntimeConfig) error {
		if err := change(cfg); err != nil {
			return err
		}
		return validateConfig(cfg)
	})
	if err != nil {
		code := http.StatusInternalServerError
		var v *validationError
		if errors.As(err, &v) {
			code = http.StatusBadRequest
		}
		if errors.Is(err, errMissing) {
			code = http.StatusNotFound
		}
		c.JSON(code, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	h.configChanged(c)
	c.JSON(status, MCPRuntimeConfigResponse{Success: true, Config: next, Enabled: h.IsMCPEnabled()})
}

func (h *Handler) CreateSource(c *gin.Context) {
	var source typ.MCPSourceConfig
	if err := c.ShouldBindJSON(&source); err != nil {
		c.JSON(400, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	if source.Origin == "" {
		source.Origin = "external"
		if source.Transport == "advisor" {
			source.Origin = "builtin"
		}
	}
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		for _, s := range cfg.Sources {
			if s.ID == source.ID {
				return invalid("source already exists: %s", s.ID)
			}
		}
		cfg.Sources = append(cfg.Sources, source)
		return nil
	}, http.StatusCreated)
}

func (h *Handler) PatchSource(c *gin.Context) {
	var patch map[string]json.RawMessage
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(400, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	id := c.Param("source_id")
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		for i, s := range cfg.Sources {
			if s.ID != id {
				continue
			}
			raw, _ := json.Marshal(s)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			for key, value := range patch {
				fields[key] = value
			}
			raw, _ = json.Marshal(fields)
			var next typ.MCPSourceConfig
			if err := json.Unmarshal(raw, &next); err != nil {
				return invalid("invalid source patch: %v", err)
			}
			if next.ID != id {
				return invalid("source ID cannot be changed")
			}
			cfg.Sources[i] = next
			return nil
		}
		return errMissing
	}, http.StatusOK)
}

func (h *Handler) DeleteSource(c *gin.Context) {
	id := c.Param("source_id")
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		found := false
		cfg.Sources = slices.DeleteFunc(cfg.Sources, func(s typ.MCPSourceConfig) bool {
			if s.ID == id {
				found = true
				return true
			}
			return false
		})
		if !found {
			return errMissing
		}
		for i := range cfg.ClientProfiles {
			cfg.ClientProfiles[i].Sources = slices.DeleteFunc(cfg.ClientProfiles[i].Sources, func(s string) bool { return s == id })
			cfg.ClientProfiles[i].Tools = slices.DeleteFunc(cfg.ClientProfiles[i].Tools, func(name string) bool { source, _, _ := mcpruntime.ParseNormalizedToolName(name); return source == id })
		}
		return nil
	}, http.StatusOK)
}

type MCPCatalogResponse struct {
	Success bool                      `json:"success"`
	Enabled bool                      `json:"enabled"`
	Sources []mcpruntime.SourceStatus `json:"sources"`
	Error   string                    `json:"error,omitempty"`
}
type MCPSourceStatusResponse struct {
	Success bool                    `json:"success"`
	Status  mcpruntime.SourceStatus `json:"status"`
	Error   string                  `json:"error,omitempty"`
}

func (h *Handler) GetCatalog(c *gin.Context) {
	if h.runtime == nil {
		c.JSON(503, MCPCatalogResponse{Error: "runtime unavailable"})
		return
	}
	c.JSON(200, MCPCatalogResponse{Success: true, Enabled: h.IsMCPEnabled(), Sources: h.runtime.Catalog(c.Request.Context(), "")})
}
func (h *Handler) CheckSource(c *gin.Context)     { h.sourceStatus(c, false) }
func (h *Handler) ReconnectSource(c *gin.Context) { h.sourceStatus(c, true) }
func (h *Handler) sourceStatus(c *gin.Context, reconnect bool) {
	if h.runtime == nil {
		c.JSON(503, MCPSourceStatusResponse{Error: "runtime unavailable"})
		return
	}
	var status mcpruntime.SourceStatus
	var err error
	if reconnect {
		status, err = h.runtime.ReconnectSource(c.Request.Context(), c.Param("source_id"))
		if h.transportHandler != nil {
			h.transportHandler.ResetAll()
		}
	} else {
		status, err = h.runtime.DiscoverSource(c.Request.Context(), c.Param("source_id"))
	}
	response := MCPSourceStatusResponse{Success: err == nil, Status: status}
	if err != nil {
		response.Error = err.Error()
	}
	code := 200
	var missing *mcpruntime.SourceNotFoundError
	if errors.As(err, &missing) {
		code = 404
	}
	c.JSON(code, response)
}

type MCPToolCallRequest struct {
	SourceID  string         `json:"source_id" binding:"required"`
	ToolName  string         `json:"tool_name" binding:"required"`
	Arguments map[string]any `json:"arguments,omitempty"`
}
type MCPToolCallResponse struct {
	Success       bool                 `json:"success"`
	Result        *coretool.ToolResult `json:"result,omitempty"`
	ExecutionTime int64                `json:"execution_time"`
	Error         string               `json:"error,omitempty"`
}

func (h *Handler) CallTool(c *gin.Context) {
	if !h.IsMCPEnabled() {
		c.JSON(403, MCPToolCallResponse{Error: "MCP execution is disabled"})
		return
	}
	if h.runtime == nil {
		c.JSON(503, MCPToolCallResponse{Error: "runtime unavailable"})
		return
	}
	var req MCPToolCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, MCPToolCallResponse{Error: err.Error()})
		return
	}
	status, err := h.runtime.DiscoverSource(c.Request.Context(), req.SourceID)
	if err != nil {
		c.JSON(200, MCPToolCallResponse{Error: err.Error()})
		return
	}
	name := ""
	for _, t := range status.Tools {
		if t.Name == req.ToolName && t.Enabled {
			name = t.NormalizedName
			break
		}
	}
	if name == "" {
		c.JSON(403, MCPToolCallResponse{Error: "tool is unavailable or disabled"})
		return
	}
	args, _ := json.Marshal(req.Arguments)
	start := time.Now()
	result, err := h.runtime.CallTool(c.Request.Context(), name, string(args))
	response := MCPToolCallResponse{Success: err == nil, ExecutionTime: time.Since(start).Milliseconds()}
	if err != nil {
		response.Error = err.Error()
	} else {
		response.Result = &result
	}
	c.JSON(200, response)
}

type MCPClientProfilesResponse struct {
	Success  bool                   `json:"success"`
	Profiles []typ.MCPClientProfile `json:"profiles"`
	Error    string                 `json:"error,omitempty"`
}

func (h *Handler) ListClientProfiles(c *gin.Context) {
	if h.cfg == nil {
		c.JSON(503, MCPClientProfilesResponse{Error: "config unavailable"})
		return
	}
	cfg := h.cfg.GetMCPRuntimeConfig()
	profiles := []typ.MCPClientProfile{}
	if cfg != nil {
		profiles = append(profiles, cfg.ClientProfiles...)
	}
	c.JSON(200, MCPClientProfilesResponse{Success: true, Profiles: profiles})
}
func (h *Handler) SaveClientProfile(c *gin.Context) {
	var p typ.MCPClientProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	if p.ID != c.Param("profile_id") {
		c.JSON(400, MCPRuntimeConfigResponse{Error: "profile ID does not match path"})
		return
	}
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		cfg.ClientProfilesConfigured = true
		for i, v := range cfg.ClientProfiles {
			if v.ID == p.ID {
				cfg.ClientProfiles[i] = p
				return nil
			}
		}
		cfg.ClientProfiles = append(cfg.ClientProfiles, p)
		return nil
	}, 200)
}
func (h *Handler) DeleteClientProfile(c *gin.Context) {
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		found := false
		cfg.ClientProfiles = slices.DeleteFunc(cfg.ClientProfiles, func(p typ.MCPClientProfile) bool {
			if p.ID == c.Param("profile_id") {
				found = true
				return true
			}
			return false
		})
		if !found {
			return errMissing
		}
		return nil
	}, 200)
}
