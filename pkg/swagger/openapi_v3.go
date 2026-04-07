package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
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
		Components: rm.buildV3Components(),
		Tags:       []Tag{},
	}

	// Add contact info if available
	if rm.swaggerInfo.Contact.Name != "" || rm.swaggerInfo.Contact.Email != "" || rm.swaggerInfo.Contact.URL != "" {
		openapi.Info.Contact = &OpenAPIContactObject{
			Name:  rm.swaggerInfo.Contact.Name,
			Email: rm.swaggerInfo.Contact.Email,
			URL:   rm.swaggerInfo.Contact.URL,
		}
	}

	// Add license info if available
	if rm.swaggerInfo.License.Name != "" || rm.swaggerInfo.License.URL != "" {
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
			openapiPath := rm.convertPathFormat(fullPath)
			method := strings.ToLower(route.Method)

			// Create path item if not exists
			if _, exists := openapi.Paths[openapiPath]; !exists {
				openapi.Paths[openapiPath] = PathItemV3{}
			}

			// Build operation
			operation := rm.buildV3Operation(route, fullPath, modelSet)

			// Add tags
			if len(operation.Tags) > 0 {
				for _, tag := range operation.Tags {
					tagSet[tag] = true
				}
			}

			// Add operation to path item
			pathItem := openapi.Paths[openapiPath]
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
			openapi.Paths[openapiPath] = pathItem
		}
	}

	// Generate tags
	for tagName := range tagSet {
		openapi.Tags = append(openapi.Tags, Tag{
			Name:        tagName,
			Description: fmt.Sprintf("Operations related to %s", tagName),
		})
	}

	// Generate all model schemas with proper handling of nested models
	processedModels := make(map[string]bool)
	allModels := make(map[string]interface{})

	// First, collect all initial models
	for modelName, model := range modelSet {
		allModels[modelName] = model
		processedModels[modelName] = false
	}

	// Then recursively collect all nested models
	for _, model := range modelSet {
		rm.collectNestedModels(model, allModels, processedModels)
	}

	// Finally, generate schemas for all collected models
	for modelName, model := range allModels {
		openapi.Components.Schemas[modelName] = rm.generateSchemaFromModelWithCacheVersion(model, allModels, VersionV3)
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(openapi, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
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

		url := fmt.Sprintf("%s://%s%s", scheme, rm.swaggerInfo.Host, rm.swaggerInfo.BasePath)
		servers = append(servers, Server{
			URL: url,
		})
	}

	return servers
}

// buildV3Components builds the components object
func (rm *RouteManager) buildV3Components() Components {
	components := Components{
		Schemas:         make(map[string]Schema),
		SecuritySchemes: make(map[string]SecuritySchemeV3),
	}

	// Add security schemes
	components.SecuritySchemes["ApiKeyAuth"] = SecuritySchemeV3{
		Type:        "apiKey",
		Name:        "Authorization",
		In:          "header",
		Description: "API key authorization header",
	}

	return components
}

// buildV3Operation builds an OpenAPI 3.0 operation from route config
func (rm *RouteManager) buildV3Operation(route RouteConfig, fullPath string, modelSet map[string]interface{}) *OperationV3 {
	operation := &OperationV3{
		OperationID: rm.generateOperationID(route.Method, rm.convertPathFormat(fullPath)),
		Summary:     route.Description,
		Description: route.Description,
		Responses:   make(map[string]ResponseV3),
		Tags:        route.Tags,
		Parameters:  []ParameterV3{},
	}

	// Set tags (fallback to group name if not specified)
	if len(operation.Tags) == 0 && len(route.Tags) == 0 {
		// Tags will be set by the caller from the group
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

	// Handle path parameters
	var pathParams []ParameterV3
	if len(route.PathParams) > 0 {
		for _, pathParam := range route.PathParams {
			pathParams = append(pathParams, ParameterV3{
				Name:        pathParam.Name,
				In:          "path",
				Description: pathParam.Description,
				Required:    true,
				Schema: &Schema{
					Type:   pathParam.Type,
					Format: pathParam.Format,
				},
			})
		}
	} else {
		// Auto-detect path params
		v2PathParams := rm.extractPathParams(route.Path)
		for _, p := range v2PathParams {
			pathParams = append(pathParams, ParameterV3{
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
	}
	operation.Parameters = append(operation.Parameters, pathParams...)

	// Handle query parameters from QueryParams
	for _, param := range route.QueryParams {
		v3Param := ParameterV3{
			Name:        param.Name,
			In:          "query",
			Description: param.Description,
			Required:    param.Required,
			Schema: &Schema{
				Type: param.Type,
			},
		}
		if param.Default != nil {
			v3Param.Schema.Default = param.Default
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
		queryParams := rm.generateQueryParametersV3(route.QueryModel)
		operation.Parameters = append(operation.Parameters, queryParams...)
	}

	// Handle request model
	if route.RequestModel != nil {
		modelName := rm.getModelNameWithFallback(route.RequestModel, route.RequestModelName, route.Method, fullPath, "Request")
		modelSet[modelName] = route.RequestModel

		if route.Method == http.MethodGet {
			// For GET requests, add query parameters
			queryParams := rm.generateQueryParametersV3(route.RequestModel)
			operation.Parameters = append(operation.Parameters, queryParams...)
		} else {
			// For POST/PUT/DELETE requests, add request body
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

	// Handle response models
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
					Schema: &Schema{
						Type: "object",
					},
				},
			},
		}
	}

	// Handle error responses
	for _, errorResp := range route.ErrorResponses {
		statusCode := fmt.Sprintf("%d", errorResp.Code)
		response := ResponseV3{
			Description: errorResp.Message,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{
						Type: "object",
					},
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

		operation.Responses[statusCode] = response
	}

	return operation
}

// generateQueryParametersV3 generates v3 query parameters from a model
func (rm *RouteManager) generateQueryParametersV3(model interface{}) []ParameterV3 {
	var parameters []ParameterV3
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

		parameters = append(parameters, ParameterV3{
			Name:        paramName,
			In:          "query",
			Description: fmt.Sprintf("Query parameter %s", paramName),
			Required:    required,
			Schema: &Schema{
				Type:   paramType.Type,
				Format: paramType.Format,
			},
		})
	}

	return parameters
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
