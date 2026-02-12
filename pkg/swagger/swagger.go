package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

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
			swaggerPath := rm.convertPathFormat(fullPath)
			method := strings.ToLower(route.Method)

			// Create path item if not exists
			if _, exists := swagger.Paths[swaggerPath]; !exists {
				swagger.Paths[swaggerPath] = PathItem{}
			}

			// Create operation
			operation := &Operation{
				OperationID: rm.generateOperationID(route.Method, swaggerPath),
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

			// Handle path parameters (add them first so they appear before query/body params)
			pathParams := rm.extractPathParams(route.Path)
			operation.Parameters = append(operation.Parameters, pathParams...)

			// Handle query parameters from QueryParams
			for _, param := range route.QueryParams {
				swaggerParam := Parameter{
					Name:        param.Name,
					In:          "query",
					Description: param.Description,
					Required:    param.Required,
					Type:        param.Type,
				}
				if param.Default != nil {
					swaggerParam.Default = param.Default
				}
				if param.Minimum != nil {
					minVal := float64(*param.Minimum)
					swaggerParam.Minimum = &minVal
				}
				if param.Maximum != nil {
					maxVal := float64(*param.Maximum)
					swaggerParam.Maximum = &maxVal
				}
				if len(param.Enum) > 0 {
					swaggerParam.Enum = param.Enum
				}
				operation.Parameters = append(operation.Parameters, swaggerParam)
			}

			// Handle query model
			if route.QueryModel != nil {
				modelName := rm.getModelNameWithFallback(route.QueryModel, route.QueryModelName, route.Method, fullPath, "Query")
				modelSet[modelName] = route.QueryModel
				operation.Parameters = append(operation.Parameters, rm.generateQueryParameters(route.QueryModel)...)
			}

			// Handle request model
			if route.RequestModel != nil {
				modelName := rm.getModelNameWithFallback(route.RequestModel, route.RequestModelName, route.Method, fullPath, "Request")
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
				modelName := rm.getModelNameWithFallback(route.ResponseModel, route.ResponseModelName, route.Method, fullPath, "Response")
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
					modelName := rm.getModelNameWithFallback(errorResp.Model, "", route.Method, fullPath, fmt.Sprintf("Error%d", errorResp.Code))
					modelSet[modelName] = errorResp.Model
					response.Schema = &Schema{
						Ref: fmt.Sprintf("#/definitions/%s", modelName),
					}
				}

				operation.Responses[statusCode] = response
			}

			// Add operation to path item based on method
			pathItem := swagger.Paths[swaggerPath]
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
			swagger.Paths[swaggerPath] = pathItem
		}
	}

	// Generate tags
	for tagName := range tagSet {
		swagger.Tags = append(swagger.Tags, Tag{
			Name:        tagName,
			Description: fmt.Sprintf("Operations related to %s", tagName),
		})
	}

	// Generate model definitions with proper handling of nested models
	// Use a map to track which models have been processed to avoid duplication
	processedModels := make(map[string]bool)
	allModels := make(map[string]interface{})

	// First, collect all initial models
	for modelName, model := range modelSet {
		allModels[modelName] = model
		processedModels[modelName] = false // Mark as collected but not processed
	}

	// Then recursively collect all nested models
	for _, model := range modelSet {
		rm.collectNestedModels(model, allModels, processedModels)
	}

	// Finally, generate schemas for all collected models
	for modelName, model := range allModels {
		swagger.Definitions[modelName] = rm.generateSchemaFromModelWithCache(model, allModels)
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// convertPathFormat converts path parameters from :param to {param} format
func (rm *RouteManager) convertPathFormat(path string) string {
	result := path
	parts := strings.Split(path, "/")

	for _, part := range parts {
		if strings.HasPrefix(part, ":") {
			paramName := strings.TrimPrefix(part, ":")
			result = strings.Replace(result, part, "{"+paramName+"}", 1)
		}
	}

	return result
}

// extractPathParams extracts path parameters from the URL path with intelligent type detection
func (rm *RouteManager) extractPathParams(path string) []Parameter {
	var params []Parameter

	// Find all path parameters like :name, :uuid, etc.
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ":") {
			paramName := strings.TrimPrefix(part, ":")
			param := Parameter{
				Name:     paramName,
				In:       "path",
				Required: true,
				Type:     "string", // Default type
			}

			// Smart type and description inference based on parameter name
			paramLower := strings.ToLower(paramName)

			switch {
			case strings.Contains(paramLower, "uuid"):
				param.Type = "string"
				param.Format = "uuid"
				param.Description = "Unique identifier (UUID)"

			case strings.Contains(paramLower, "id") && !strings.Contains(paramLower, "providerid"):
				param.Type = "string"
				if strings.Contains(paramLower, "userid") {
					param.Description = "User ID"
				} else if strings.Contains(paramLower, "ruleid") {
					param.Description = "Rule ID"
				} else {
					param.Description = "Resource ID"
				}

			case paramLower == "name" || strings.Contains(paramLower, "provider"):
				param.Type = "string"
				param.Description = "Resource name"

			case strings.Contains(paramLower, "num") || strings.Contains(paramLower, "count"):
				param.Type = "integer"
				param.Format = "int64"
				param.Description = "Numeric count"

			case paramLower == "page":
				param.Type = "integer"
				param.Format = "int32"
				param.Description = "Page number"

			case paramLower == "size" || paramLower == "limit":
				param.Type = "integer"
				param.Format = "int32"
				param.Description = "Page size limit"

			case strings.Contains(paramLower, "timestamp") || strings.Contains(paramLower, "time"):
				param.Type = "string"
				param.Format = "date-time"
				param.Description = "Timestamp"

			case strings.Contains(paramLower, "date"):
				param.Type = "string"
				param.Format = "date"
				param.Description = "Date"

			case strings.Contains(paramLower, "email"):
				param.Type = "string"
				param.Format = "email"
				param.Description = "Email address"

			case strings.Contains(paramLower, "url") || strings.Contains(paramLower, "uri"):
				param.Type = "string"
				param.Format = "uri"
				param.Description = "URL/URI"

			default:
				param.Description = fmt.Sprintf("Path parameter '%s'", paramName)
			}

			params = append(params, param)
		}
	}

	return params
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

// SetupSwaggerEndpoints configures Swagger documentation endpoints based on environment
func (rm *RouteManager) SetupSwaggerEndpoints() {
	// only for non production env
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
