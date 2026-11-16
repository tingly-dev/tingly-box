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
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type validationError struct{ message string }

func (e *validationError) Error() string       { return e.message }
func invalid(format string, args ...any) error { return &validationError{fmt.Sprintf(format, args...)} }

var githubRepoRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?/[A-Za-z0-9._-]+$`)

var validID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
var errMissing = errors.New("MCP entry not found")

func validateConfig(cfg *typ.MCPRuntimeConfig) error { return validateConfigChange(nil, cfg) }

// validateConfigChange validates cfg. Environment references are only checked
// for sources that differ from prev, so an unrelated edit is not blocked by a
// pre-existing source whose variable is missing.
func validateConfigChange(prev, cfg *typ.MCPRuntimeConfig) error {
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
		if s.Preset != "" && s.Preset != mcptools.GitHubPreset {
			return invalid("unknown preset for %s: %s", s.ID, s.Preset)
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
	checked := cfg.Sources
	if prev != nil {
		before := map[string][]byte{}
		for _, old := range prev.Sources {
			raw, _ := json.Marshal(old)
			before[old.ID] = raw
		}
		checked = nil
		for _, source := range cfg.Sources {
			if raw, _ := json.Marshal(source); string(raw) != string(before[source.ID]) {
				checked = append(checked, source)
			}
		}
	}
	if issues := mcpruntime.ValidateEnabledMCPSourceEnvRefs(checked); len(issues) > 0 {
		return invalid("source %s: missing environment variable %s for %s", issues[0].SourceID, issues[0].VarName, issues[0].FieldPath)
	}
	seen = map[string]bool{}
	for i := range cfg.Profiles {
		p := &cfg.Profiles[i]
		if p.Tools == nil {
			p.Tools = []typ.MCPProfileTool{}
		}
		if !validID.MatchString(p.ID) || strings.Contains(p.ID, "__") {
			return invalid("invalid profile ID: %s", p.ID)
		}
		if seen[p.ID] {
			return invalid("duplicate profile ID: %s", p.ID)
		}
		seen[p.ID] = true
		if p.ID == typ.MCPDefaultProfileID {
			p.Name = "Default"
		}
		for _, repo := range p.GitHubRepos {
			if !githubRepoRE.MatchString(repo) {
				return invalid("profile %s: %q is not an owner/repo", p.ID, repo)
			}
		}
		tools := map[string]bool{}
		for _, tool := range p.Tools {
			if _, _, ok := mcpruntime.ParseNormalizedToolName(tool.Name); !ok {
				return invalid("profile %s requires normalized tool names", p.ID)
			}
			if tools[tool.Name] {
				return invalid("profile %s lists %s twice", p.ID, tool.Name)
			}
			tools[tool.Name] = true
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
		var prev typ.MCPRuntimeConfig
		raw, _ := json.Marshal(cfg)
		_ = json.Unmarshal(raw, &prev)
		if err := change(cfg); err != nil {
			return err
		}
		return validateConfigChange(&prev, cfg)
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
			// The preset is what keeps a GitHub source to its fixed subset.
			if next.Preset != s.Preset {
				return invalid("source preset cannot be changed")
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
		for i := range cfg.Profiles {
			cfg.Profiles[i].Tools = slices.DeleteFunc(cfg.Profiles[i].Tools, func(tool typ.MCPProfileTool) bool {
				source, _, _ := mcpruntime.ParseNormalizedToolName(tool.Name)
				return source == id
			})
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
	if h.runtime == nil {
		c.JSON(503, MCPToolCallResponse{Error: "runtime unavailable"})
		return
	}
	var req MCPToolCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, MCPToolCallResponse{Error: err.Error()})
		return
	}
	// The MCP flag gates connected MCP tools; Server Tools run on the gateway
	// and are testable without it.
	if !h.IsMCPEnabled() && !h.isServerToolSource(req.SourceID) {
		c.JSON(403, MCPToolCallResponse{Error: "MCP execution is disabled"})
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

type MCPProfilesResponse struct {
	Success  bool             `json:"success"`
	Profiles []typ.MCPProfile `json:"profiles"`
	Error    string           `json:"error,omitempty"`
}

func (h *Handler) ListProfiles(c *gin.Context) {
	if h.cfg == nil {
		c.JSON(503, MCPProfilesResponse{Error: "config unavailable"})
		return
	}
	cfg := h.cfg.GetMCPRuntimeConfig()
	profiles := []typ.MCPProfile{}
	if cfg != nil {
		profiles = mcpruntime.Profiles(cfg)
	}
	c.JSON(200, MCPProfilesResponse{Success: true, Profiles: profiles})
}
func (h *Handler) SaveProfile(c *gin.Context) {
	var p typ.MCPProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, MCPRuntimeConfigResponse{Error: err.Error()})
		return
	}
	if p.ID != c.Param("profile_id") {
		c.JSON(400, MCPRuntimeConfigResponse{Error: "profile ID does not match path"})
		return
	}
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		for i, v := range cfg.Profiles {
			if v.ID == p.ID {
				cfg.Profiles[i] = p
				return nil
			}
		}
		cfg.Profiles = append(cfg.Profiles, p)
		return nil
	}, 200)
}
func (h *Handler) DeleteProfile(c *gin.Context) {
	if c.Param("profile_id") == typ.MCPDefaultProfileID {
		c.JSON(http.StatusBadRequest, MCPRuntimeConfigResponse{Error: "the Default profile cannot be deleted"})
		return
	}
	h.mutate(c, func(cfg *typ.MCPRuntimeConfig) error {
		found := false
		cfg.Profiles = slices.DeleteFunc(cfg.Profiles, func(p typ.MCPProfile) bool {
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

// isServerToolSource reports whether the source is a virtual Server Tool source.
func (h *Handler) isServerToolSource(id string) bool {
	if h.cfg == nil {
		return false
	}
	cfg := h.cfg.GetMCPRuntimeConfig()
	if cfg == nil {
		return false
	}
	for _, source := range cfg.Sources {
		if source.ID == id {
			return mcpruntime.IsServerToolSource(source)
		}
	}
	return false
}
