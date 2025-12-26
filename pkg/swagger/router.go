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
	Method         string
	Path           string
	Handler        Handler
	Description    string
	Tags           []string
	AuthRequired   bool
	RequestModel   interface{} // For swagger documentation
	ResponseModel  interface{} // For swagger documentation
	ErrorResponses []ErrorResponseConfig
	Middleware     []gin.HandlerFunc
	Deprecated     bool
	DeprecatedMsg  string
}

// ErrorResponseConfig defines error response configuration for swagger
type ErrorResponseConfig struct {
	Code    int
	Message string
	Model   interface{}
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
	groups      map[string]*RouteGroup
	globalMW    []gin.HandlerFunc
	swaggerInfo *SwaggerInfo
}

// NewRouteManager creates a new route manager
func NewRouteManager(engine *gin.Engine) *RouteManager {
	return &RouteManager{
		engine: engine,
		groups: make(map[string]*RouteGroup),
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

	rm.groups[fullPrefix] = group
	return group
}

// AddMiddleware adds middleware to the route group
func (rg *RouteGroup) AddMiddleware(middleware ...gin.HandlerFunc) {
	rg.middleware = append(rg.middleware, middleware...)
	for _, mw := range rg.middleware {
		rg.Router.Use(mw)
	}
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

	// Add route-specific middleware
	middleware = append(middleware, config.Middleware...)

	// Register route with gin and add the handler
	middleware = append(middleware, func(c *gin.Context) {
		config.Handler(c)
	})

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
func WithRequestModel(model interface{}) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.RequestModel = model
	}
}

// WithResponseModel sets the response model for swagger documentation
func WithResponseModel(model interface{}) func(*RouteConfig) {
	return func(rc *RouteConfig) {
		rc.ResponseModel = model
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
func (rm *RouteManager) GetRouteGroups() map[string]*RouteGroup {
	return rm.groups
}

func (rm *RouteManager) GetEngine() *gin.Engine {
	return rm.engine
}

// GenerateSwaggerAnnotations generates swagger annotations for all routes
func (rm *RouteManager) GenerateSwaggerAnnotations() string {
	var annotations strings.Builder

	// Add main swagger annotation
	annotations.WriteString(`// @title ` + rm.swaggerInfo.Title + "\n")
	annotations.WriteString(`// @version ` + rm.swaggerInfo.Version + "\n")
	annotations.WriteString(`// @description ` + rm.swaggerInfo.Description + "\n")

	if rm.swaggerInfo.Host != "" {
		annotations.WriteString(`// @host ` + rm.swaggerInfo.Host + "\n")
	}

	if rm.swaggerInfo.BasePath != "" {
		annotations.WriteString(`// @BasePath ` + rm.swaggerInfo.BasePath + "\n")
	}

	annotations.WriteString("\n")

	// Add contact and license if available
	if rm.swaggerInfo.Contact.Name != "" {
		annotations.WriteString(`// @contact.name ` + rm.swaggerInfo.Contact.Name + "\n")
		if rm.swaggerInfo.Contact.Email != "" {
			annotations.WriteString(`// @contact.email ` + rm.swaggerInfo.Contact.Email + "\n")
		}
		if rm.swaggerInfo.Contact.URL != "" {
			annotations.WriteString(`// @contact.url ` + rm.swaggerInfo.Contact.URL + "\n")
		}
	}

	if rm.swaggerInfo.License.Name != "" {
		annotations.WriteString(`// @license.name ` + rm.swaggerInfo.License.Name + "\n")
		if rm.swaggerInfo.License.URL != "" {
			annotations.WriteString(`// @license.url ` + rm.swaggerInfo.License.URL + "\n")
		}
	}

	annotations.WriteString("\n")

	// Generate annotations for each route
	for _, group := range rm.groups {
		for _, route := range group.routes {
			annotations.WriteString(rm.generateRouteAnnotations(group, route))
			annotations.WriteString("\n")
		}
	}

	return annotations.String()
}

// generateRouteAnnotations generates swagger annotations for a specific route
func (rm *RouteManager) generateRouteAnnotations(group *RouteGroup, route RouteConfig) string {
	var annotations strings.Builder

	// Add basic route information
	annotations.WriteString("// @Summary " + route.Description + "\n")
	annotations.WriteString("// @Description " + route.Description + "\n")

	// Add tags
	if len(route.Tags) > 0 {
		for _, tag := range route.Tags {
			annotations.WriteString("// @Tags " + tag + "\n")
		}
	} else {
		annotations.WriteString("// @Tags " + group.name + "\n")
	}

	// Add HTTP method and path
	fullPath := group.prefix + route.Path
	swaggerPath := rm.convertPathFormat(fullPath)
	methodLower := strings.ToLower(route.Method)
	annotations.WriteString("// @Router " + swaggerPath + "[" + methodLower + "]\n")

	// Add request model if specified
	if route.RequestModel != nil {
		modelName := getModelName(route.RequestModel)
		if route.Method == http.MethodGet {
			annotations.WriteString("// @Param " + modelName + " query " + modelName + " true \"Request parameters\"\n")
		} else {
			annotations.WriteString("// @Param request body " + modelName + " true \"Request body\"\n")
		}
	}

	// Add response model if specified
	if route.ResponseModel != nil {
		modelName := getModelName(route.ResponseModel)
		annotations.WriteString("// @Success 200 {object} " + modelName + "\n")
	} else {
		annotations.WriteString("// @Success 200 {object} map[string]interface{}\n")
	}

	// Add error responses
	for _, errorResp := range route.ErrorResponses {
		if errorResp.Model != nil {
			modelName := getModelName(errorResp.Model)
			annotations.WriteString("// @Failure " + string(rune(errorResp.Code)) + " {object} " + modelName + "\n")
		} else {
			annotations.WriteString("// @Failure " + string(rune(errorResp.Code)) + " {object} map[string]interface{}\n")
		}
	}

	// Add security if auth required
	if route.AuthRequired {
		annotations.WriteString("// @Security ApiKeyAuth\n")
	}

	// Add deprecated if specified
	if route.Deprecated {
		if route.DeprecatedMsg != "" {
			annotations.WriteString("// @Deprecated " + route.DeprecatedMsg + "\n")
		} else {
			annotations.WriteString("// @Deprecated\n")
		}
	}

	return annotations.String()
}
