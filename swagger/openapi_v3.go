package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GenerateOpenAPI generates OpenAPI specification (v2 or v3)
func (rm *RouteManager) GenerateOpenAPI(version Version) (string, error) {
	switch version {
	case VersionV3:
		return rm.buildV3Spec()
	default:
		return rm.GenerateSwaggerJSON()
	}
}

// buildV3Spec builds the complete OpenAPI 3.0 specification
func (rm *RouteManager) buildV3Spec() (string, error) {
	openapi := &OpenAPI{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfoObject{
			Title:       rm.swaggerInfo.Title,
			Description: rm.swaggerInfo.Description,
			Version:     rm.swaggerInfo.Version,
		},
		Servers:    rm.buildV3Servers(),
		Paths:      make(map[string]PathItemV3),
		Components: buildV3Components(),
	}

	// Add contact info if available
	if rm.swaggerInfo.Contact != (SwaggerContact{}) {
		openapi.Info.Contact = &OpenAPIContactObject{
			Name:  rm.swaggerInfo.Contact.Name,
			Email: rm.swaggerInfo.Contact.Email,
			URL:   rm.swaggerInfo.Contact.URL,
		}
	}

	// Add license info if available
	if rm.swaggerInfo.License != (SwaggerLicense{}) {
		openapi.Info.License = &OpenAPILicenseObject{
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
			openapiPath := convertPathFormat(fullPath)

			operation := rm.buildV3Operation(group, route, fullPath, modelSet)
			for _, tag := range operation.Tags {
				tagSet[tag] = true
			}

			pathItem := openapi.Paths[openapiPath]
			setV3Operation(&pathItem, route.Method, operation)
			openapi.Paths[openapiPath] = pathItem
		}
	}

	openapi.Tags = sortedTags(tagSet)

	// Generate schemas for registered models and all nested models
	openapi.Components.Schemas = newSchemaGen(VersionV3).buildDefinitions(modelSet)

	jsonData, err := json.MarshalIndent(openapi, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// setV3Operation assigns an operation to the method slot of a path item
func setV3Operation(pathItem *PathItemV3, method string, operation *OperationV3) {
	switch strings.ToLower(method) {
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
}

// buildV3Servers builds server list from swagger info
func (rm *RouteManager) buildV3Servers() []Server {
	servers := []Server{}

	// Build server URL from host, basePath, and schemes
	if rm.swaggerInfo.Host != "" {
		// Default to https if no schemes specified
		scheme := "https"
		if len(rm.swaggerInfo.Schemes) > 0 {
			scheme = rm.swaggerInfo.Schemes[0]
		}

		servers = append(servers, Server{
			URL: fmt.Sprintf("%s://%s%s", scheme, rm.swaggerInfo.Host, rm.swaggerInfo.BasePath),
		})
	}

	return servers
}

// buildV3Components builds the components object
func buildV3Components() Components {
	return Components{
		SecuritySchemes: map[string]SecuritySchemeV3{
			"ApiKeyAuth": {
				Type:        "apiKey",
				Name:        "Authorization",
				In:          "header",
				Description: "API key authorization header",
			},
		},
	}
}

// buildV3Operation builds an OpenAPI 3.0 operation from route config
func (rm *RouteManager) buildV3Operation(group *RouteGroup, route RouteConfig, fullPath string, modelSet map[string]interface{}) *OperationV3 {
	operation := &OperationV3{
		OperationID: generateOperationID(route.Method, convertPathFormat(fullPath)),
		Summary:     route.Description,
		Description: route.Description,
		Responses:   make(map[string]ResponseV3),
		Tags:        route.Tags,
		Parameters:  []ParameterV3{},
	}

	// Fallback to group name when the route has no tags
	if len(operation.Tags) == 0 {
		operation.Tags = []string{group.name}
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

	// Path parameters first so they appear before query params
	for _, p := range resolvePathParams(route) {
		operation.Parameters = append(operation.Parameters, ParameterV3{
			Name:        p.Name,
			In:          "path",
			Description: p.Description,
			Required:    true,
			Schema: &Schema{
				Type:   p.Type,
				Format: p.Format,
			},
		})
	}

	// Handle query parameters from QueryParams
	for _, param := range route.QueryParams {
		v3Param := ParameterV3{
			Name:        param.Name,
			In:          "query",
			Description: param.Description,
			Required:    param.Required,
			Schema: &Schema{
				Type:    normalizeSchemaType(param.Type),
				Default: param.Default,
			},
		}
		if param.Minimum != nil {
			minVal := float64(*param.Minimum)
			v3Param.Schema.Minimum = &minVal
		}
		if param.Maximum != nil {
			maxVal := float64(*param.Maximum)
			v3Param.Schema.Maximum = &maxVal
		}
		if len(param.Enum) > 0 {
			v3Param.Schema.Enum = param.Enum
		}
		operation.Parameters = append(operation.Parameters, v3Param)
	}

	// Handle query model
	if route.QueryModel != nil {
		modelName := rm.getModelNameWithFallback(route.QueryModel, route.QueryModelName, route.Method, fullPath, "Query")
		modelSet[modelName] = route.QueryModel
		for _, qp := range modelQueryParams(route.QueryModel) {
			operation.Parameters = append(operation.Parameters, qp.toV3())
		}
	}

	// Handle request model
	if route.RequestModel != nil {
		modelName := rm.getModelNameWithFallback(route.RequestModel, route.RequestModelName, route.Method, fullPath, "Request")
		modelSet[modelName] = route.RequestModel

		if route.Method == http.MethodGet {
			// For GET requests, document the model as query parameters
			for _, qp := range modelQueryParams(route.RequestModel) {
				operation.Parameters = append(operation.Parameters, qp.toV3())
			}
		} else {
			operation.RequestBody = &RequestBody{
				Description: "Request body",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Ref: "#/components/schemas/" + modelName,
						},
					},
				},
			}
		}
	}

	// Handle response model
	if route.ResponseModel != nil {
		modelName := rm.getModelNameWithFallback(route.ResponseModel, route.ResponseModelName, route.Method, fullPath, "Response")
		modelSet[modelName] = route.ResponseModel
		operation.Responses["200"] = ResponseV3{
			Description: "Successful response",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{
						Ref: "#/components/schemas/" + modelName,
					},
				},
			},
		}
	} else {
		operation.Responses["200"] = ResponseV3{
			Description: "Successful response",
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Type: "object"},
				},
			},
		}
	}

	// Handle error responses
	for _, errorResp := range route.ErrorResponses {
		response := ResponseV3{
			Description: errorResp.Message,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{Type: "object"},
				},
			},
		}
		if errorResp.Model != nil {
			modelName := rm.getModelNameWithFallback(errorResp.Model, "", route.Method, fullPath, fmt.Sprintf("Error%d", errorResp.Code))
			modelSet[modelName] = errorResp.Model
			response.Content["application/json"] = MediaType{
				Schema: &Schema{
					Ref: "#/components/schemas/" + modelName,
				},
			}
		}
		operation.Responses[fmt.Sprintf("%d", errorResp.Code)] = response
	}

	return operation
}

// SetupOpenAPIEndpoints configures both Swagger v2 and OpenAPI v3 documentation endpoints
func (rm *RouteManager) SetupOpenAPIEndpoints() {
	// only for non production env
	if gin.Mode() != gin.ReleaseMode {
		rm.RegisterOpenAPIEndpoint("/swagger.json", VersionV2)
		rm.RegisterOpenAPIEndpoint("/openapi.json", VersionV3)
	}
}

// RegisterOpenAPIEndpoint registers an OpenAPI endpoint with specified version
func (rm *RouteManager) RegisterOpenAPIEndpoint(path string, version Version) {
	rm.engine.GET(path, func(c *gin.Context) {
		openapiJSON, err := rm.GenerateOpenAPI(version)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to generate OpenAPI documentation",
				"details": err.Error(),
			})
			return
		}

		c.Header("Content-Type", "application/json")
		c.String(http.StatusOK, openapiJSON)
	})
}

// normalizeSchemaType normalizes Go type names to OpenAPI schema types
func normalizeSchemaType(goType string) string {
	switch goType {
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "integer"
	case "float", "float32", "float64":
		return "number"
	case "string":
		return "string"
	default:
		// Return as-is for complex types or already-correct types
		return goType
	}
}
