package swagger

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler defines the interface for all API handlers
type Handler func(c *gin.Context)

// RouteConfig defines configuration for a single route
type RouteConfig struct {
	Method            string
	Path              string
	Handler           Handler
	Description       string
	Tags              []string
	AuthRequired      bool
	RequestModel      interface{} // For swagger documentation
	ResponseModel     interface{} // For swagger documentation
	RequestModelName  string      // Optional explicit name for anonymous request struct
	ResponseModelName string      // Optional explicit name for anonymous response struct
	ErrorResponses    []ErrorResponseConfig
	Middleware        []gin.HandlerFunc
	Deprecated        bool
	DeprecatedMsg     string
	QueryParams       []QueryParamConfig // Query parameters
	QueryModel        interface{}        // Query model for swagger documentation
	QueryModelName    string             // Optional explicit name for anonymous query struct
	PathParams        []PathParamConfig  // Path parameters (overrides auto-detected)
}

// ErrorResponseConfig defines error response configuration for swagger
type ErrorResponseConfig struct {
	Code    int
	Message string
	Model   interface{}
}

// QueryParamConfig defines configuration for a single query parameter
type QueryParamConfig struct {
	Name        string
	Type        string
	Required    bool
	Default     interface{}
	Description string
	Enum        []interface{}
	Minimum     *int
	Maximum     *int
}

// PathParamConfig defines configuration for a single path parameter
type PathParamConfig struct {
	Name        string
	Type        string
	Format      string
	Description string
}

// RouteGroup manages a group of related routes
type RouteGroup struct {
	name       string
	version    string
	prefix     string
	tags       []string
	Router     *gin.RouterGroup
	routes     []RouteConfig
	middleware []gin.HandlerFunc
}

// RouteGroupOption is a function that configures a RouteGroup
type RouteGroupOption func(*RouteGroup)

// RouteManager manages all route groups and swagger generation
type RouteManager struct {
	engine      *gin.Engine
	groups      []*RouteGroup
	globalMW    []gin.HandlerFunc
	swaggerInfo *SwaggerInfo
}

// NewRouteManager creates a new route manager
func NewRouteManager(engine *gin.Engine) *RouteManager {
	return &RouteManager{
		engine: engine,
		groups: make([]*RouteGroup, 0),
		swaggerInfo: &SwaggerInfo{
			Title:       "API Documentation",
			Description: "Generated API documentation",
			Version:     "1.0.0",
		},
	}
}

// SetSwaggerInfo sets the swagger information
func (rm *RouteManager) SetSwaggerInfo(info SwaggerInfo) {
	rm.swaggerInfo = &info
}

// AddGlobalMiddleware adds middleware to all routes
func (rm *RouteManager) AddGlobalMiddleware(middleware ...gin.HandlerFunc) {
	rm.globalMW = append(rm.globalMW, middleware...)
}

// NewGroup creates a new route group
func (rm *RouteManager) NewGroup(name, version, prefix string, opts ...RouteGroupOption) *RouteGroup {
	fullPrefix := fmt.Sprintf("/%s/%s", name, version)
	if prefix != "" {
		fullPrefix += "/" + strings.TrimPrefix(prefix, "/")
	}

	ginGroup := rm.engine.Group(fullPrefix)

	// Apply global middleware
	for _, mw := range rm.globalMW {
		ginGroup.Use(mw)
	}

	group := &RouteGroup{
		name:       name,
		version:    version,
		prefix:     fullPrefix,
		tags:       []string{name}, // Default to group name as tag
		Router:     ginGroup,
		routes:     make([]RouteConfig, 0),
		middleware: make([]gin.HandlerFunc, 0),
	}

	// Apply group options
	for _, opt := range opts {
		opt(group)
	}

	rm.groups = append(rm.groups, group)
	return group
}

// AddMiddleware adds middleware to the route group
func (rg *RouteGroup) AddMiddleware(middleware ...gin.HandlerFunc) {
	rg.middleware = append(rg.middleware, middleware...)
	rg.Router.Use(middleware...)
}

// RegisterRoute registers a single route
func (rg *RouteGroup) RegisterRoute(config RouteConfig) {
	// If route doesn't have tags, inherit from group
	if len(config.Tags) == 0 && len(rg.tags) > 0 {
		config.Tags = make([]string, len(rg.tags))
		copy(config.Tags, rg.tags)
	}

	// Build middleware chain
	var middleware []gin.HandlerFunc

	// Add auth middleware if required
	if config.AuthRequired {
		middleware = append(middleware, rg.authMiddleware())
	}

	// Add route-specific middleware, then the handler itself
	middleware = append(middleware, config.Middleware...)
	middleware = append(middleware, gin.HandlerFunc(config.Handler))

	rg.Router.Handle(config.Method, config.Path, middleware...)

	// Store route configuration for swagger generation
	rg.routes = append(rg.routes, config)
}

// GET is a shortcut for RegisterRoute with GET method
func (rg *RouteGroup) GET(path string, handler Handler, options ...func(*RouteConfig)) {
	config := RouteConfig{
		Method:  http.MethodGet,
		Path:    path,
		Handler: handler,
	}

	for _, option := range options {
		option(&config)
	}

	rg.RegisterRoute(config)
}

// POST is a shortcut for RegisterRoute with POST method
func (rg *RouteGroup) POST(path string, handler Handler, options ...func(*RouteConfig)) {
	config := RouteConfig{
		Method:  http.MethodPost,
		Path:    path,
		Handler: handler,
	}

	for _, option := range options {
		option(&config)
	}

	rg.RegisterRoute(config)
}

// PUT is a shortcut for RegisterRoute with PUT method
func (rg *RouteGroup) PUT(path string, handler Handler, options ...func(*RouteConfig)) {
	config := RouteConfig{
		Method:  http.MethodPut,
		Path:    path,
		Handler: handler,
	}

	for _, option := range options {
		option(&config)
	}

	rg.RegisterRoute(config)
}

// DELETE is a shortcut for RegisterRoute with DELETE method
func (rg *RouteGroup) DELETE(path string, handler Handler, options ...func(*RouteConfig)) {
	config := RouteConfig{
		Method:  http.MethodDelete,
		Path:    path,
		Handler: handler,
	}

	for _, option := range options {
		option(&config)
	}

	rg.RegisterRoute(config)
}

// PATCH is a shortcut for RegisterRoute with PATCH method
func (rg *RouteGroup) PATCH(path string, handler Handler, options ...func(*RouteConfig)) {
	config := RouteConfig{
		Method:  http.MethodPatch,
		Path:    path,
		Handler: handler,
	}

	for _, option := range options {
		option(&config)
	}

	rg.RegisterRoute(config)
}

// Route configuration options

// GroupWithTags sets the tags for all routes in the group (unless overridden)
func GroupWithTags(tags ...string) RouteGroupOption {
	return func(rg *RouteGroup) {
		rg.tags = tags
	}
}

// WithDescription sets the route description
func WithDescription(desc string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.Description = desc
	}
}

// WithTags sets the route tags for swagger grouping
func WithTags(tags ...string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.Tags = tags
	}
}

// WithAuth marks the route as requiring authentication
func WithAuth() func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.AuthRequired = true
	}
}

// WithRequestModel sets the request model for swagger documentation
// Optional name parameter can be provided to specify an explicit model name for anonymous structs
func WithRequestModel(model interface{}, name ...string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.RequestModel = model
		if len(name) > 0 {
			rc.RequestModelName = name[0]
		}
	}
}

// WithResponseModel sets the response model for swagger documentation
// Optional name parameter can be provided to specify an explicit model name for anonymous structs
func WithResponseModel(model interface{}, name ...string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.ResponseModel = model
		if len(name) > 0 {
			rc.ResponseModelName = name[0]
		}
	}
}

// WithErrorResponses sets the error responses for swagger documentation
func WithErrorResponses(errors ...ErrorResponseConfig) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.ErrorResponses = errors
	}
}

// WithMiddleware adds route-specific middleware
func WithMiddleware(middleware ...gin.HandlerFunc) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.Middleware = middleware
	}
}

// WithDeprecated marks the route as deprecated
func WithDeprecated(message string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.Deprecated = true
		rc.DeprecatedMsg = message
	}
}

// WithQuery adds a simple query parameter (optional by default)
func WithQuery(name, paramType, description string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.QueryParams = append(rc.QueryParams, QueryParamConfig{
			Name:        name,
			Type:        paramType,
			Required:    false,
			Description: description,
		})
	}
}

// WithQueryRequired adds a required query parameter
func WithQueryRequired(name, paramType, description string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.QueryParams = append(rc.QueryParams, QueryParamConfig{
			Name:        name,
			Type:        paramType,
			Required:    true,
			Description: description,
		})
	}
}

// WithQueryConfig adds a query parameter with full configuration
func WithQueryConfig(name string, config QueryParamConfig) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		config.Name = name
		rc.QueryParams = append(rc.QueryParams, config)
	}
}

// WithQueryModel sets a query model for swagger documentation
// Optional name parameter can be provided to specify an explicit model name for anonymous structs
func WithQueryModel(model interface{}, name ...string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.QueryModel = model
		if len(name) > 0 {
			rc.QueryModelName = name[0]
		}
	}
}

// WithRequestModelName sets an explicit name for the request model (useful for anonymous structs)
func WithRequestModelName(name string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.RequestModelName = name
	}
}

// WithResponseModelName sets an explicit name for the response model (useful for anonymous structs)
func WithResponseModelName(name string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.ResponseModelName = name
	}
}

// WithQueryModelName sets an explicit name for the query model (useful for anonymous structs)
func WithQueryModelName(name string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.QueryModelName = name
	}
}

// WithPathParam adds a path parameter with explicit configuration (overrides auto-detection)
func WithPathParam(name, paramType, description string) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.PathParams = append(rc.PathParams, PathParamConfig{
			Name:        name,
			Type:        paramType,
			Description: description,
		})
	}
}

// WithPathParamConfig adds a path parameter with full configuration
func WithPathParamConfig(config PathParamConfig) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.PathParams = append(rc.PathParams, config)
	}
}

// authMiddleware is a placeholder for authentication middleware
func (rg *RouteGroup) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement actual authentication logic
		// For now, just check for Authorization header
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetRouteGroups returns all registered route groups
func (rm *RouteManager) GetRouteGroups() []*RouteGroup {
	return rm.groups
}

func (rm *RouteManager) GetEngine() *gin.Engine {
	return rm.engine
}
