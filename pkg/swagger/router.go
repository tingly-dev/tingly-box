package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
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
	Router     *gin.RouterGroup
	routes     []RouteConfig
	middleware []gin.HandlerFunc
}

// RouteManager manages all route groups and swagger generation
type RouteManager struct {
	engine      *gin.Engine
	groups      map[string]*RouteGroup
	globalMW    []gin.HandlerFunc
	swaggerInfo *SwaggerInfo
}

// SwaggerInfo holds information for swagger documentation
type SwaggerInfo struct {
	Title          string
	Description    string
	Version        string
	Host           string
	BasePath       string
	TermsOfService string
	Contact        SwaggerContact
	License        SwaggerLicense
}

// SwaggerContact holds contact information
type SwaggerContact struct {
	Name  string
	Email string
	URL   string
}

// SwaggerLicense holds license information
type SwaggerLicense struct {
	Name string
	URL  string
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
func (rm *RouteManager) NewGroup(name, version, prefix string) *RouteGroup {
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
		Router:     ginGroup,
		routes:     make([]RouteConfig, 0),
		middleware: make([]gin.HandlerFunc, 0),
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
	methodLower := strings.ToLower(route.Method)
	annotations.WriteString("// @Router " + fullPath + "[" + methodLower + "]\n")

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

// getModelName extracts the model name from the struct type
func getModelName(model interface{}) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// Swagger represents the complete Swagger specification
type Swagger struct {
	Swagger             string                    `json:"swagger"`
	Info                SwaggerInfoObject         `json:"info"`
	Host                string                    `json:"host,omitempty"`
	BasePath            string                    `json:"basePath,omitempty"`
	Schemes             []string                  `json:"schemes,omitempty"`
	Paths               map[string]PathItem       `json:"paths"`
	Definitions         map[string]Schema         `json:"definitions,omitempty"`
	SecurityDefinitions map[string]SecurityScheme `json:"securityDefinitions,omitempty"`
	Tags                []Tag                     `json:"tags,omitempty"`
	ExternalDocs        *ExternalDocs             `json:"externalDocs,omitempty"`
}

// SwaggerInfoObject represents the info object in Swagger
type SwaggerInfoObject struct {
	Title          string                `json:"title"`
	Description    string                `json:"description,omitempty"`
	TermsOfService string                `json:"termsOfService,omitempty"`
	Contact        *SwaggerContactObject `json:"contact,omitempty"`
	License        *SwaggerLicenseObject `json:"license,omitempty"`
	Version        string                `json:"version"`
}

// SwaggerContactObject represents contact information
type SwaggerContactObject struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// SwaggerLicenseObject represents license information
type SwaggerLicenseObject struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// PathItem represents a path item object in Swagger
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
}

// Operation represents an operation object in Swagger
type Operation struct {
	Tags         []string              `json:"tags,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	Description  string                `json:"description,omitempty"`
	ExternalDocs *ExternalDocs         `json:"externalDocs,omitempty"`
	OperationID  string                `json:"operationId,omitempty"`
	Consumes     []string              `json:"consumes,omitempty"`
	Produces     []string              `json:"produces,omitempty"`
	Parameters   []Parameter           `json:"parameters,omitempty"`
	Responses    map[string]Response   `json:"responses"`
	Schemes      []string              `json:"schemes,omitempty"`
	Deprecated   bool                  `json:"deprecated,omitempty"`
	Security     []map[string][]string `json:"security,omitempty"`
}

// Parameter represents a parameter object in Swagger
type Parameter struct {
	Name             string        `json:"name"`
	In               string        `json:"in"` // "query", "header", "path", "formData", "body"
	Description      string        `json:"description,omitempty"`
	Required         bool          `json:"required"`
	Schema           *Schema       `json:"schema,omitempty"` // For body parameters
	Type             string        `json:"type,omitempty"`   // For non-body parameters
	Format           string        `json:"format,omitempty"`
	AllowEmptyValue  bool          `json:"allowEmptyValue,omitempty"`
	Items            *Items        `json:"items,omitempty"` // For array parameters
	CollectionFormat string        `json:"collectionFormat,omitempty"`
	Default          interface{}   `json:"default,omitempty"`
	Maximum          *float64      `json:"maximum,omitempty"`
	Minimum          *float64      `json:"minimum,omitempty"`
	MaxLength        *int          `json:"maxLength,omitempty"`
	MinLength        *int          `json:"minLength,omitempty"`
	Pattern          string        `json:"pattern,omitempty"`
	MaxItems         *int          `json:"maxItems,omitempty"`
	MinItems         *int          `json:"minItems,omitempty"`
	UniqueItems      bool          `json:"uniqueItems,omitempty"`
	Enum             []interface{} `json:"enum,omitempty"`
	MultipleOf       *float64      `json:"multipleOf,omitempty"`
}

// Items represents items object for array parameters
type Items struct {
	Type             string        `json:"type,omitempty"`
	Format           string        `json:"format,omitempty"`
	Items            *Items        `json:"items,omitempty"`
	CollectionFormat string        `json:"collectionFormat,omitempty"`
	Default          interface{}   `json:"default,omitempty"`
	Maximum          *float64      `json:"maximum,omitempty"`
	Minimum          *float64      `json:"minimum,omitempty"`
	MaxLength        *int          `json:"maxLength,omitempty"`
	MinLength        *int          `json:"minLength,omitempty"`
	Pattern          string        `json:"pattern,omitempty"`
	MaxItems         *int          `json:"maxItems,omitempty"`
	MinItems         *int          `json:"minItems,omitempty"`
	UniqueItems      bool          `json:"uniqueItems,omitempty"`
	Enum             []interface{} `json:"enum,omitempty"`
	MultipleOf       *float64      `json:"multipleOf,omitempty"`
}

// Response represents a response object in Swagger
type Response struct {
	Description string                 `json:"description"`
	Schema      *Schema                `json:"schema,omitempty"`
	Headers     map[string]Header      `json:"headers,omitempty"`
	Examples    map[string]interface{} `json:"examples,omitempty"`
}

// Header represents a header object in Swagger
type Header struct {
	Description      string        `json:"description,omitempty"`
	Type             string        `json:"type"`
	Format           string        `json:"format,omitempty"`
	Items            *Items        `json:"items,omitempty"`
	CollectionFormat string        `json:"collectionFormat,omitempty"`
	Default          interface{}   `json:"default,omitempty"`
	Maximum          *float64      `json:"maximum,omitempty"`
	Minimum          *float64      `json:"minimum,omitempty"`
	MaxLength        *int          `json:"maxLength,omitempty"`
	MinLength        *int          `json:"minLength,omitempty"`
	Pattern          string        `json:"pattern,omitempty"`
	MaxItems         *int          `json:"maxItems,omitempty"`
	MinItems         *int          `json:"minItems,omitempty"`
	UniqueItems      bool          `json:"uniqueItems,omitempty"`
	Enum             []interface{} `json:"enum,omitempty"`
	MultipleOf       *float64      `json:"multipleOf,omitempty"`
}

// Schema represents a schema object in Swagger
type Schema struct {
	Type                 string            `json:"type,omitempty"`
	Format               string            `json:"format,omitempty"`
	Title                string            `json:"title,omitempty"`
	Description          string            `json:"description,omitempty"`
	Default              interface{}       `json:"default,omitempty"`
	MultipleOf           *float64          `json:"multipleOf,omitempty"`
	Maximum              *float64          `json:"maximum,omitempty"`
	ExclusiveMaximum     bool              `json:"exclusiveMaximum,omitempty"`
	Minimum              *float64          `json:"minimum,omitempty"`
	ExclusiveMinimum     bool              `json:"exclusiveMinimum,omitempty"`
	MaxLength            *int              `json:"maxLength,omitempty"`
	MinLength            *int              `json:"minLength,omitempty"`
	Pattern              string            `json:"pattern,omitempty"`
	MaxItems             *int              `json:"maxItems,omitempty"`
	MinItems             *int              `json:"minItems,omitempty"`
	UniqueItems          bool              `json:"uniqueItems,omitempty"`
	Enum                 []interface{}     `json:"enum,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
	AllOf                []Schema          `json:"allOf,omitempty"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	AdditionalProperties *Schema           `json:"additionalProperties,omitempty"`
	ReadOnly             bool              `json:"readOnly,omitempty"`
	XML                  *XML              `json:"xml,omitempty"`
	ExternalDocs         *ExternalDocs     `json:"externalDocs,omitempty"`
	Example              interface{}       `json:"example,omitempty"`
	Required             []string          `json:"required,omitempty"`
	Ref                  string            `json:"$ref,omitempty"`
}

// XML represents XML object in Swagger
type XML struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
}

// ExternalDocs represents external documentation object
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// SecurityScheme represents a security scheme in Swagger
type SecurityScheme struct {
	Type             string            `json:"type"`
	Description      string            `json:"description,omitempty"`
	Name             string            `json:"name,omitempty"`
	In               string            `json:"in,omitempty"`   // "query", "header"
	Flow             string            `json:"flow,omitempty"` // "implicit", "password", "application", "accessCode"
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

// Tag represents a tag object in Swagger
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// GenerateSwaggerJSON generates the complete Swagger JSON specification
func (rm *RouteManager) GenerateSwaggerJSON() (string, error) {
	swagger := &Swagger{
		Swagger: "2.0",
		Info: SwaggerInfoObject{
			Title:       rm.swaggerInfo.Title,
			Description: rm.swaggerInfo.Description,
			Version:     rm.swaggerInfo.Version,
		},
		Host:        rm.swaggerInfo.Host,
		BasePath:    rm.swaggerInfo.BasePath,
		Schemes:     []string{"http", "https"},
		Paths:       make(map[string]PathItem),
		Definitions: make(map[string]Schema),
		SecurityDefinitions: map[string]SecurityScheme{
			"ApiKeyAuth": {
				Type:        "apiKey",
				Name:        "Authorization",
				In:          "header",
				Description: "API key authorization header",
			},
		},
		Tags: []Tag{},
	}

	// Add contact info if available
	if rm.swaggerInfo.Contact.Name != "" || rm.swaggerInfo.Contact.Email != "" || rm.swaggerInfo.Contact.URL != "" {
		swagger.Info.Contact = &SwaggerContactObject{
			Name:  rm.swaggerInfo.Contact.Name,
			Email: rm.swaggerInfo.Contact.Email,
			URL:   rm.swaggerInfo.Contact.URL,
		}
	}

	// Add license info if available
	if rm.swaggerInfo.License.Name != "" || rm.swaggerInfo.License.URL != "" {
		swagger.Info.License = &SwaggerLicenseObject{
			Name: rm.swaggerInfo.License.Name,
			URL:  rm.swaggerInfo.License.URL,
		}
	}

	// Process all routes
	tagSet := make(map[string]bool)
	modelSet := make(map[string]interface{})

	for _, group := range rm.groups {
		for _, route := range group.routes {
			fullPath := group.prefix + route.Path
			method := strings.ToLower(route.Method)

			// Create path item if not exists
			if _, exists := swagger.Paths[fullPath]; !exists {
				swagger.Paths[fullPath] = PathItem{}
			}

			// Create operation
			operation := &Operation{
				Summary:     route.Description,
				Description: route.Description,
				Responses:   make(map[string]Response),
				Consumes:    []string{"application/json"},
				Produces:    []string{"application/json"},
			}

			// Set tags
			if len(route.Tags) > 0 {
				operation.Tags = route.Tags
				for _, tag := range route.Tags {
					tagSet[tag] = true
				}
			} else {
				operation.Tags = []string{group.name}
				tagSet[group.name] = true
			}

			// Handle deprecated
			if route.Deprecated {
				operation.Deprecated = true
				if route.DeprecatedMsg != "" {
					operation.Description = fmt.Sprintf("%s\n\nDEPRECATED: %s", route.Description, route.DeprecatedMsg)
				}
			}

			// Handle authentication
			if route.AuthRequired {
				operation.Security = []map[string][]string{
					{"ApiKeyAuth": {}},
				}
			}

			// Handle request model
			if route.RequestModel != nil {
				modelName := getModelName(route.RequestModel)
				modelSet[modelName] = route.RequestModel

				if route.Method == http.MethodGet {
					// For GET requests, add query parameters
					operation.Parameters = append(operation.Parameters, rm.generateQueryParameters(route.RequestModel)...)
				} else {
					// For POST/PUT/DELETE requests, add body parameter
					operation.Parameters = append(operation.Parameters, Parameter{
						Name:        "request",
						In:          "body",
						Description: "Request body",
						Required:    true,
						Schema: &Schema{
							Ref: fmt.Sprintf("#/definitions/%s", modelName),
						},
					})
				}
			}

			// Handle response models
			if route.ResponseModel != nil {
				modelName := getModelName(route.ResponseModel)
				modelSet[modelName] = route.ResponseModel
				operation.Responses["200"] = Response{
					Description: "Successful response",
					Schema: &Schema{
						Ref: fmt.Sprintf("#/definitions/%s", modelName),
					},
				}
			} else {
				operation.Responses["200"] = Response{
					Description: "Successful response",
					Schema: &Schema{
						Type: "object",
					},
				}
			}

			// Handle error responses
			for _, errorResp := range route.ErrorResponses {
				statusCode := fmt.Sprintf("%d", errorResp.Code)
				response := Response{
					Description: errorResp.Message,
					Schema: &Schema{
						Type: "object",
					},
				}

				if errorResp.Model != nil {
					modelName := getModelName(errorResp.Model)
					modelSet[modelName] = errorResp.Model
					response.Schema = &Schema{
						Ref: fmt.Sprintf("#/definitions/%s", modelName),
					}
				}

				operation.Responses[statusCode] = response
			}

			// Add operation to path item based on method
			pathItem := swagger.Paths[fullPath]
			switch method {
			case "get":
				pathItem.Get = operation
			case "post":
				pathItem.Post = operation
			case "put":
				pathItem.Put = operation
			case "delete":
				pathItem.Delete = operation
			case "patch":
				pathItem.Patch = operation
			case "options":
				pathItem.Options = operation
			case "head":
				pathItem.Head = operation
			}
			swagger.Paths[fullPath] = pathItem
		}
	}

	// Generate tags
	for tagName := range tagSet {
		swagger.Tags = append(swagger.Tags, Tag{
			Name:        tagName,
			Description: fmt.Sprintf("Operations related to %s", tagName),
		})
	}

	// Generate model definitions
	for modelName, model := range modelSet {
		swagger.Definitions[modelName] = rm.generateSchemaFromModel(model)
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// generateQueryParameters generates query parameters from a model
func (rm *RouteManager) generateQueryParameters(model interface{}) []Parameter {
	var parameters []Parameter
	modelType := reflect.TypeOf(model)

	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	if modelType.Kind() != reflect.Struct {
		return parameters
	}

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldName := field.Name
		jsonTag := field.Tag.Get("json")
		formTag := field.Tag.Get("form")

		// Determine the parameter name
		paramName := fieldName
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				paramName = parts[0]
			}
		} else if formTag != "" && formTag != "-" {
			parts := strings.Split(formTag, ",")
			if parts[0] != "" {
				paramName = parts[0]
			}
		}

		// Check if field is required
		bindingTag := field.Tag.Get("binding")
		required := strings.Contains(bindingTag, "required")

		// Determine parameter type
		paramType := rm.getSwaggerType(field.Type)

		// Create parameter
		param := Parameter{
			Name:        paramName,
			In:          "query",
			Description: fmt.Sprintf("Query parameter %s", paramName),
			Required:    required,
			Type:        paramType.Type,
			Format:      paramType.Format,
		}

		parameters = append(parameters, param)
	}

	return parameters
}

// generateSchemaFromModel generates Swagger schema from a Go struct
func (rm *RouteManager) generateSchemaFromModel(model interface{}) Schema {
	modelType := reflect.TypeOf(model)

	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	if modelType.Kind() != reflect.Struct {
		// Handle primitive types
		return rm.getSwaggerType(modelType)
	}

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldName := field.Name

		// Get all relevant tags
		jsonTag := field.Tag.Get("json")
		bindingTag := field.Tag.Get("binding")
		exampleTag := field.Tag.Get("example")
		defaultTag := field.Tag.Get("default")
		docTag := field.Tag.Get("doc")
		descriptionTag := field.Tag.Get("description")
		formatTag := field.Tag.Get("format")
		enumTag := field.Tag.Get("enum")

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Determine the property name and json options
		propName := fieldName
		isOptional := false
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
			// Check for omitempty and other options
			for _, part := range parts[1:] {
				switch part {
				case "omitempty":
					isOptional = true
				case "stringize":
					// Handle string conversion
				case "omitempty,stringize":
					isOptional = true
				}
			}
		}

		// Generate property schema with type-specific details
		propSchema := rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)

		// Set description with priority
		if descriptionTag != "" {
			propSchema.Description = descriptionTag
		} else if docTag != "" {
			propSchema.Description = docTag
		} else {
			propSchema.Description = fmt.Sprintf("Field %s", propName)
		}

		// Set example value
		if exampleTag != "" {
			propSchema.Example = rm.parseExampleValue(exampleTag, field.Type)
		}

		// Set default value
		if defaultTag != "" {
			propSchema.Default = rm.parseDefaultValue(defaultTag, field.Type)
		}

		// Parse binding validation rules
		rm.parseValidationRules(&propSchema, bindingTag, field.Type)

		// Check if field is required (binding tag takes precedence over omitempty)
		if strings.Contains(bindingTag, "required") {
			schema.Required = append(schema.Required, propName)
		} else if !isOptional {
			// Add to required if not explicitly optional
			schema.Required = append(schema.Required, propName)
		}

		schema.Properties[propName] = propSchema
	}

	return schema
}

// getSwaggerTypeWithDetails converts Go type to Swagger schema with additional details
func (rm *RouteManager) getSwaggerTypeWithDetails(goType reflect.Type, bindingTag, formatTag, enumTag string) Schema {
	schema := rm.getSwaggerType(goType)

	// Override format if explicitly specified
	if formatTag != "" {
		schema.Format = formatTag
	}

	// Handle enum values
	if enumTag != "" {
		enumValues := strings.Split(enumTag, ",")
		schema.Enum = make([]interface{}, len(enumValues))
		for i, val := range enumValues {
			schema.Enum[i] = rm.parseEnumValue(val, goType)
		}
	}

	return schema
}

// parseValidationRules extracts validation constraints from binding tag
func (rm *RouteManager) parseValidationRules(schema *Schema, bindingTag string, goType reflect.Type) {
	if bindingTag == "" {
		return
	}

	rules := strings.Split(bindingTag, ",")
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)

		switch {
		case strings.HasPrefix(rule, "min="):
			if minVal := rm.parseNumericValue(strings.TrimPrefix(rule, "min="), goType); minVal != nil {
				if goType.Kind() == reflect.String {
					schema.MinLength = minVal.(*int)
				} else {
					schema.Minimum = minVal.(*float64)
				}
			}
		case strings.HasPrefix(rule, "max="):
			if maxVal := rm.parseNumericValue(strings.TrimPrefix(rule, "max="), goType); maxVal != nil {
				if goType.Kind() == reflect.String {
					schema.MaxLength = maxVal.(*int)
				} else {
					schema.Maximum = maxVal.(*float64)
				}
			}
		case strings.HasPrefix(rule, "len="):
			if lenVal := rm.parseNumericValue(strings.TrimPrefix(rule, "len="), goType); lenVal != nil {
				if goType.Kind() == reflect.String || goType.Kind() == reflect.Slice || goType.Kind() == reflect.Array {
					schema.MinLength = lenVal.(*int)
					schema.MaxLength = lenVal.(*int)
				}
			}
		case strings.HasPrefix(rule, "gt="):
			if gtVal := rm.parseNumericValue(strings.TrimPrefix(rule, "gt="), goType); gtVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					minVal := float64(*gtVal.(*float64)) + 0.000001
					schema.Minimum = &minVal
					schema.ExclusiveMinimum = true
				} else {
					minVal := float64(*gtVal.(*int64)) + 1
					schema.Minimum = &minVal
					schema.ExclusiveMinimum = true
				}
			}
		case strings.HasPrefix(rule, "gte="):
			if gteVal := rm.parseNumericValue(strings.TrimPrefix(rule, "gte="), goType); gteVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					minVal := float64(*gteVal.(*float64))
					schema.Minimum = &minVal
				} else {
					minVal := float64(*gteVal.(*int64))
					schema.Minimum = &minVal
				}
			}
		case strings.HasPrefix(rule, "lt="):
			if ltVal := rm.parseNumericValue(strings.TrimPrefix(rule, "lt="), goType); ltVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					maxVal := float64(*ltVal.(*float64)) - 0.000001
					schema.Maximum = &maxVal
					schema.ExclusiveMaximum = true
				} else {
					maxVal := float64(*ltVal.(*int64)) - 1
					schema.Maximum = &maxVal
					schema.ExclusiveMaximum = true
				}
			}
		case strings.HasPrefix(rule, "lte="):
			if lteVal := rm.parseNumericValue(strings.TrimPrefix(rule, "lte="), goType); lteVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					maxVal := float64(*lteVal.(*float64))
					schema.Maximum = &maxVal
				} else {
					maxVal := float64(*lteVal.(*int64))
					schema.Maximum = &maxVal
				}
			}
		case rule == "email":
			schema.Format = "email"
		case rule == "url":
			schema.Format = "uri"
		case rule == "uri":
			schema.Format = "uri"
		case rule == "uuid":
			schema.Format = "uuid"
		case rule == "date":
			schema.Format = "date"
		case rule == "datetime":
			schema.Format = "date-time"
		case strings.HasPrefix(rule, "oneof="):
			enumValues := strings.Split(strings.TrimPrefix(rule, "oneof="), " ")
			schema.Enum = make([]interface{}, len(enumValues))
			for i, val := range enumValues {
				schema.Enum[i] = rm.parseEnumValue(val, goType)
			}
		case strings.HasPrefix(rule, "alpha"):
			schema.Pattern = "^[a-zA-Z]+$"
		case strings.HasPrefix(rule, "alphanum"):
			schema.Pattern = "^[a-zA-Z0-9]+$"
		case strings.HasPrefix(rule, "numeric"):
			schema.Pattern = "^[0-9]+$"
		case strings.HasPrefix(rule, "hexadecimal"):
			schema.Pattern = "^[0-9a-fA-F]+$"
		case strings.HasPrefix(rule, "contains="):
			value := strings.TrimPrefix(rule, "contains=")
			if goType.Kind() == reflect.String {
				schema.Pattern = ".*" + value + ".*"
			}
		case strings.HasPrefix(rule, "startswith="):
			value := strings.TrimPrefix(rule, "startswith=")
			if goType.Kind() == reflect.String {
				schema.Pattern = "^" + value + ".*"
			}
		case strings.HasPrefix(rule, "endswith="):
			value := strings.TrimPrefix(rule, "endswith=")
			if goType.Kind() == reflect.String {
				schema.Pattern = ".*" + value + "$"
			}
		}
	}
}

// parseNumericValue parses numeric values from string tags
func (rm *RouteManager) parseNumericValue(value string, goType reflect.Type) interface{} {
	if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			return &val
		}
	} else {
		if val, err := strconv.ParseInt(value, 10, 64); err == nil {
			return &val
		}
	}
	return nil
}

// parseExampleValue parses example value based on field type
func (rm *RouteManager) parseExampleValue(value string, goType reflect.Type) interface{} {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

	switch goType.Kind() {
	case reflect.String:
		return value
	case reflect.Bool:
		if val, err := strconv.ParseBool(value); err == nil {
			return val
		}
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val, err := strconv.ParseInt(value, 10, 64); err == nil {
			return val
		}
		return int64(0)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val, err := strconv.ParseUint(value, 10, 64); err == nil {
			return val
		}
		return uint64(0)
	case reflect.Float32, reflect.Float64:
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			return val
		}
		return 0.0
	case reflect.Slice, reflect.Array:
		if goType.Elem().Kind() == reflect.String {
			return strings.Split(value, ",")
		}
		return []interface{}{}
	default:
		return value
	}
}

// parseDefaultValue parses default value based on field type
func (rm *RouteManager) parseDefaultValue(value string, goType reflect.Type) interface{} {
	return rm.parseExampleValue(value, goType)
}

// parseEnumValue parses enum value based on field type
func (rm *RouteManager) parseEnumValue(value string, goType reflect.Type) interface{} {
	return rm.parseExampleValue(value, goType)
}

// getSwaggerType converts Go type to Swagger schema
func (rm *RouteManager) getSwaggerType(goType reflect.Type) Schema {
	switch goType.Kind() {
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Schema{Type: "integer", Format: "int64"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer", Format: "int64"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number", Format: "double"}
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		itemSchema := rm.getSwaggerType(goType.Elem())
		return Schema{
			Type:  "array",
			Items: &itemSchema,
		}
	case reflect.Map:
		return Schema{Type: "object"}
	case reflect.Struct:
		// Handle time.Time specially
		if goType.String() == "time.Time" {
			return Schema{Type: "string", Format: "date-time"}
		}
		return Schema{Type: "object"}
	case reflect.Ptr:
		return rm.getSwaggerType(goType.Elem())
	case reflect.Interface:
		return Schema{Type: "object"}
	default:
		return Schema{Type: "object"}
	}
}

// SetupSwaggerEndpoints configures Swagger documentation endpoints based on environment
func (rm *RouteManager) SetupSwaggerEndpoints() {
	// 只在非生产环境启用
	if gin.Mode() != gin.ReleaseMode {
		rm.RegisterSwaggerEndpoint("/swagger.json")
	}
}

// RegisterSwaggerEndpoint registers the swagger JSON endpoint
func (rm *RouteManager) RegisterSwaggerEndpoint(path string) {
	rm.engine.GET(path, func(c *gin.Context) {
		swaggerJSON, err := rm.GenerateSwaggerJSON()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to generate swagger documentation",
				"details": err.Error(),
			})
			return
		}

		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, swaggerJSON)
	})
}
