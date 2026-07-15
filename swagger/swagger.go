package swagger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// GenerateSwaggerJSON generates the complete Swagger 2.0 JSON specification
func (rm *RouteManager) GenerateSwaggerJSON() (string, error) {
	schemes := rm.swaggerInfo.Schemes
	if len(schemes) == 0 {
		schemes = []string{"http", "https"}
	}

	swagger := &Swagger{
		Swagger: "2.0",
		Info: SwaggerInfoObject{
			Title:       rm.swaggerInfo.Title,
			Description: rm.swaggerInfo.Description,
			Version:     rm.swaggerInfo.Version,
		},
		Host:     rm.swaggerInfo.Host,
		BasePath: rm.swaggerInfo.BasePath,
		Schemes:  schemes,
		Paths:    make(map[string]PathItem),
		SecurityDefinitions: map[string]SecurityScheme{
			"ApiKeyAuth": {
				Type:        "apiKey",
				Name:        "Authorization",
				In:          "header",
				Description: "API key authorization header",
			},
		},
	}

	// Add contact info if available
	if rm.swaggerInfo.Contact != (SwaggerContact{}) {
		swagger.Info.Contact = &SwaggerContactObject{
			Name:  rm.swaggerInfo.Contact.Name,
			Email: rm.swaggerInfo.Contact.Email,
			URL:   rm.swaggerInfo.Contact.URL,
		}
	}

	// Add license info if available
	if rm.swaggerInfo.License != (SwaggerLicense{}) {
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
			swaggerPath := convertPathFormat(fullPath)

			operation := rm.buildV2Operation(group, route, fullPath, swaggerPath, modelSet)
			for _, tag := range operation.Tags {
				tagSet[tag] = true
			}

			pathItem := swagger.Paths[swaggerPath]
			setV2Operation(&pathItem, route.Method, operation)
			swagger.Paths[swaggerPath] = pathItem
		}
	}

	swagger.Tags = sortedTags(tagSet)

	// Generate definitions for registered models and all nested models
	swagger.Definitions = newSchemaGen(VersionV2).buildDefinitions(modelSet)

	jsonData, err := json.MarshalIndent(swagger, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// buildV2Operation builds a Swagger 2.0 operation from route config
func (rm *RouteManager) buildV2Operation(group *RouteGroup, route RouteConfig, fullPath, swaggerPath string, modelSet map[string]interface{}) *Operation {
	operation := &Operation{
		OperationID: generateOperationID(route.Method, swaggerPath),
		Summary:     route.Description,
		Description: route.Description,
		Responses:   make(map[string]Response),
		Consumes:    []string{"application/json"},
		Produces:    []string{"application/json"},
	}

	// Set tags (fallback to group name)
	if len(route.Tags) > 0 {
		operation.Tags = route.Tags
	} else {
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

	// Path parameters first so they appear before query/body params.
	// Explicit path params override auto-detection from the route path.
	operation.Parameters = append(operation.Parameters, resolvePathParams(route)...)

	// Handle query parameters from QueryParams
	for _, param := range route.QueryParams {
		swaggerParam := Parameter{
			Name:        param.Name,
			In:          "query",
			Description: param.Description,
			Required:    param.Required,
			Type:        normalizeSchemaType(param.Type),
			Default:     param.Default,
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
		for _, qp := range modelQueryParams(route.QueryModel) {
			operation.Parameters = append(operation.Parameters, qp.toV2())
		}
	}

	// Handle request model
	if route.RequestModel != nil {
		modelName := rm.getModelNameWithFallback(route.RequestModel, route.RequestModelName, route.Method, fullPath, "Request")
		modelSet[modelName] = route.RequestModel

		if route.Method == http.MethodGet {
			// For GET requests, document the model as query parameters
			for _, qp := range modelQueryParams(route.RequestModel) {
				operation.Parameters = append(operation.Parameters, qp.toV2())
			}
		} else {
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

	// Handle response model
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
			Schema:      &Schema{Type: "object"},
		}
	}

	// Handle error responses
	for _, errorResp := range route.ErrorResponses {
		response := Response{
			Description: errorResp.Message,
			Schema:      &Schema{Type: "object"},
		}
		if errorResp.Model != nil {
			modelName := rm.getModelNameWithFallback(errorResp.Model, "", route.Method, fullPath, fmt.Sprintf("Error%d", errorResp.Code))
			modelSet[modelName] = errorResp.Model
			response.Schema = &Schema{
				Ref: fmt.Sprintf("#/definitions/%s", modelName),
			}
		}
		operation.Responses[fmt.Sprintf("%d", errorResp.Code)] = response
	}

	return operation
}

// setV2Operation assigns an operation to the method slot of a path item
func setV2Operation(pathItem *PathItem, method string, operation *Operation) {
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

// sortedTags converts a tag name set into a deterministic, sorted tag list.
// Map iteration order is random; without sorting, every generation shuffles
// the tags section and produces spurious diffs in committed specs.
func sortedTags(tagSet map[string]bool) []Tag {
	names := make([]string, 0, len(tagSet))
	for name := range tagSet {
		names = append(names, name)
	}
	sort.Strings(names)

	tags := make([]Tag, 0, len(names))
	for _, name := range names {
		tags = append(tags, Tag{
			Name:        name,
			Description: fmt.Sprintf("Operations related to %s", name),
		})
	}
	return tags
}

// resolvePathParams returns the path parameters for a route: explicitly
// configured ones if present, otherwise auto-detected from the route path.
func resolvePathParams(route RouteConfig) []Parameter {
	if len(route.PathParams) == 0 {
		return extractPathParams(route.Path)
	}

	params := make([]Parameter, 0, len(route.PathParams))
	for _, pathParam := range route.PathParams {
		params = append(params, Parameter{
			Name:        pathParam.Name,
			In:          "path",
			Description: pathParam.Description,
			Required:    true,
			Type:        pathParam.Type,
			Format:      pathParam.Format,
		})
	}
	return params
}

// convertPathFormat converts path parameters from :param to {param} format
func convertPathFormat(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

// extractPathParams extracts path parameters from the URL path with intelligent type detection
func extractPathParams(path string) []Parameter {
	var params []Parameter

	// Find all path parameters like :name, :uuid, etc.
	for _, part := range strings.Split(path, "/") {
		if !strings.HasPrefix(part, ":") {
			continue
		}
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
			param.Format = "uuid"
			param.Description = "Unique identifier (UUID)"

		case strings.Contains(paramLower, "id") && !strings.Contains(paramLower, "providerid"):
			if strings.Contains(paramLower, "userid") {
				param.Description = "User ID"
			} else if strings.Contains(paramLower, "ruleid") {
				param.Description = "Rule ID"
			} else {
				param.Description = "Resource ID"
			}

		case paramLower == "name" || strings.Contains(paramLower, "provider"):
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
			param.Format = "date-time"
			param.Description = "Timestamp"

		case strings.Contains(paramLower, "date"):
			param.Format = "date"
			param.Description = "Date"

		case strings.Contains(paramLower, "email"):
			param.Format = "email"
			param.Description = "Email address"

		case strings.Contains(paramLower, "url") || strings.Contains(paramLower, "uri"):
			param.Format = "uri"
			param.Description = "URL/URI"

		default:
			param.Description = fmt.Sprintf("Path parameter '%s'", paramName)
		}

		params = append(params, param)
	}

	return params
}

// queryParamSpec is a version-neutral description of a query parameter
// derived from a model field.
type queryParamSpec struct {
	Name        string
	Description string
	Required    bool
	Type        string
	Format      string
}

func (q queryParamSpec) toV2() Parameter {
	return Parameter{
		Name:        q.Name,
		In:          "query",
		Description: q.Description,
		Required:    q.Required,
		Type:        q.Type,
		Format:      q.Format,
	}
}

func (q queryParamSpec) toV3() ParameterV3 {
	return ParameterV3{
		Name:        q.Name,
		In:          "query",
		Description: q.Description,
		Required:    q.Required,
		Schema: &Schema{
			Type:   q.Type,
			Format: q.Format,
		},
	}
}

// modelQueryParams derives query parameters from a model's exported fields.
// The parameter name follows gin's query binding: form tag first, then json
// tag as documentation fallback, then the field name. Fields opting out with
// "-" are skipped. Embedded structs are flattened.
func modelQueryParams(model interface{}) []queryParamSpec {
	modelType := derefType(reflect.TypeOf(model))
	if modelType.Kind() != reflect.Struct {
		return nil
	}
	return structQueryParams(modelType)
}

func structQueryParams(modelType reflect.Type) []queryParamSpec {
	var parameters []queryParamSpec

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		if field.PkgPath != "" {
			continue
		}

		formName := tagName(field.Tag.Get("form"))
		jsonName := tagName(field.Tag.Get("json"))
		if formName == "-" || (formName == "" && jsonName == "-") {
			continue
		}

		// Flatten embedded structs the same way gin's form binding does
		if field.Anonymous && formName == "" && jsonName == "" {
			embeddedType := derefType(field.Type)
			if embeddedType.Kind() == reflect.Struct && embeddedType != timeType {
				parameters = append(parameters, structQueryParams(embeddedType)...)
				continue
			}
		}

		paramName := field.Name
		if formName != "" {
			paramName = formName
		} else if jsonName != "" {
			paramName = jsonName
		}

		paramType, paramFormat := queryParamType(field.Type)

		parameters = append(parameters, queryParamSpec{
			Name:        paramName,
			Description: fmt.Sprintf("Query parameter %s", paramName),
			Required:    hasValidationRule(field.Tag.Get("binding"), "required"),
			Type:        paramType,
			Format:      paramFormat,
		})
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
