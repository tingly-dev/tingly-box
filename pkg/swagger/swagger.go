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
			swaggerPath := rm.convertPathFormat(fullPath)
			method := strings.ToLower(route.Method)

			// Create path item if not exists
			if _, exists := swagger.Paths[swaggerPath]; !exists {
				swagger.Paths[swaggerPath] = PathItem{}
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
				modelName := getModelName(route.QueryModel)
				modelSet[modelName] = route.QueryModel
				operation.Parameters = append(operation.Parameters, rm.generateQueryParameters(route.QueryModel)...)
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

// generateSchemaFromModel generates Swagger schema from a Go struct
func (rm *RouteManager) generateSchemaFromModel(model interface{}) Schema {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// Check if this is a primitive type
	if modelType.Kind() != reflect.Struct {
		return rm.getSwaggerType(modelType)
	}

	// For struct, generate schema with references to nested models
	return rm.generateSchemaWithReferences(model)
}

// generateSchemaWithReferences generates schema using $ref for known nested models
func (rm *RouteManager) generateSchemaWithReferences(model interface{}) Schema {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
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
		var propSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			// For nested structs, create a reference
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// For anonymous structs, generate inline schema
				collectedModels := make(map[string]interface{})
				propSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// For named structs, use $ref
				propSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			// Handle slices/arrays
			elemType := field.Type.Elem()
			// Check if element is a pointer to struct
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModelName := elemType.Name()
				if nestedModelName == "" {
					// For anonymous struct elements in array
					collectedModels := make(map[string]interface{})
					itemSchema := rm.generateAnonymousStructSchema(elemType, collectedModels)
					propSchema = Schema{
						Type:  "array",
						Items: &itemSchema,
					}
				} else {
					// For named struct elements in array
					propSchema = Schema{
						Type:  "array",
						Items: &Schema{Ref: fmt.Sprintf("#/definitions/%s", nestedModelName)},
					}
				}
			} else {
				propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
			}
		} else {
			propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
		}

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

// generateSchemaFromModelWithDefinitions generates Swagger schema and collects nested models
func (rm *RouteManager) generateSchemaFromModelWithDefinitions(model interface{}, collectedModels map[string]interface{}) Schema {
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
		var propSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			// For nested structs, create a reference
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// For anonymous structs, generate inline schema
				propSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// For named structs, collect them and use $ref
				if _, exists := collectedModels[nestedModelName]; !exists {
					// Create a zero value of the struct to collect it
					nestedModel := reflect.Zero(fieldType).Interface()
					collectedModels[nestedModelName] = nestedModel
				}
				propSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			// Handle slices/arrays
			elemType := field.Type.Elem()
			// Check if element is a pointer to struct
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModelName := elemType.Name()
				if nestedModelName == "" {
					// For anonymous struct elements in array
					itemSchema := rm.generateAnonymousStructSchema(elemType, collectedModels)
					propSchema = Schema{
						Type:  "array",
						Items: &itemSchema,
					}
				} else {
					// For named struct elements in array
					if _, exists := collectedModels[nestedModelName]; !exists {
						nestedModel := reflect.Zero(elemType).Interface()
						collectedModels[nestedModelName] = nestedModel
					}
					propSchema = Schema{
						Type:  "array",
						Items: &Schema{Ref: fmt.Sprintf("#/definitions/%s", nestedModelName)},
					}
				}
			} else {
				propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
			}
		} else {
			propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
		}

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

// generateAnonymousStructSchema generates schema for an anonymous struct
func (rm *RouteManager) generateAnonymousStructSchema(structType reflect.Type, collectedModels map[string]interface{}) Schema {
	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Get field name from json tag
		propName := field.Name
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
		}

		// Generate schema for this field
		var fieldSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// Anonymous struct inside anonymous struct
				fieldSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// Named struct inside anonymous struct
				if _, exists := collectedModels[nestedModelName]; !exists {
					nestedModel := reflect.Zero(fieldType).Interface()
					collectedModels[nestedModelName] = nestedModel
				}
				fieldSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else {
			fieldSchema = rm.getSwaggerType(field.Type)
		}

		// Add description
		descriptionTag := field.Tag.Get("description")
		docTag := field.Tag.Get("doc")
		if descriptionTag != "" {
			fieldSchema.Description = descriptionTag
		} else if docTag != "" {
			fieldSchema.Description = docTag
		} else {
			fieldSchema.Description = fmt.Sprintf("Field %s", propName)
		}

		schema.Properties[propName] = fieldSchema

		// Check if field is required
		if !strings.Contains(jsonTag, "omitempty") {
			schema.Required = append(schema.Required, propName)
		}
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
				} else if fv, ok := minVal.(*float64); ok {
					schema.Minimum = fv
				} else if iv, ok := minVal.(*int64); ok {
					minVal := float64(*iv)
					schema.Minimum = &minVal
				}
			}
		case strings.HasPrefix(rule, "max="):
			if maxVal := rm.parseNumericValue(strings.TrimPrefix(rule, "max="), goType); maxVal != nil {
				if goType.Kind() == reflect.String {
					schema.MaxLength = maxVal.(*int)
				} else if fv, ok := maxVal.(*float64); ok {
					schema.Maximum = fv
				} else if iv, ok := maxVal.(*int64); ok {
					maxVal := float64(*iv)
					schema.Maximum = &maxVal
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
		elemType := goType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		// Just return basic array type, nested structs will be handled at higher level
		return Schema{
			Type:  "array",
			Items: &Schema{Type: elemType.Name()},
		}
	case reflect.Map:
		// Get key and value types
		valueType := goType.Elem()

		if valueType.Kind() == reflect.Ptr {
			valueType = valueType.Elem()
		}

		// Generate schema for map with additionalProperties
		schema := Schema{Type: "object"}

		// Set value type in additionalProperties
		if valueType.Kind() == reflect.Struct && valueType.String() != "time.Time" {
			// For struct values, use reference
			schema.AdditionalProperties = &Schema{
				Ref: fmt.Sprintf("#/definitions/%s", valueType.Name()),
			}
		} else {
			// For primitive types, get the swagger type
			valueSchema := rm.getSwaggerType(valueType)
			schema.AdditionalProperties = &valueSchema
		}

		return schema
	case reflect.Struct:
		// Handle time.Time specially
		if goType.String() == "time.Time" {
			return Schema{Type: "string", Format: "date-time"}
		}
		// For nested structs, check if it's a known model type
		// If it's a primitive wrapper or basic type, return object
		// Otherwise, we'll handle it in generateSchemaFromModel
		return Schema{Type: "object"}
	case reflect.Ptr:
		return rm.getSwaggerType(goType.Elem())
	case reflect.Interface:
		return Schema{Type: "object"}
	default:
		return Schema{Type: "object"}
	}
}

// collectNestedModels recursively collects all nested struct models
func (rm *RouteManager) collectNestedModels(model interface{}, allModels map[string]interface{}, processedModels map[string]bool) {
	modelType := reflect.TypeOf(model)

	// Handle pointer types
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// Only process struct types (except time.Time)
	if modelType.Kind() != reflect.Struct || modelType.String() == "time.Time" {
		return
	}

	modelName := modelType.Name()

	// Skip anonymous structs (they don't have names)
	if modelName == "" {
		return
	}

	// If already processed, skip
	if processed, exists := processedModels[modelName]; exists && processed {
		return
	}

	// Mark as being processed to avoid infinite recursion
	processedModels[modelName] = true

	// Add to allModels if not already there
	if _, exists := allModels[modelName]; !exists {
		allModels[modelName] = reflect.Zero(modelType).Interface()
	}

	// Recursively process all fields
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Process field type
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// If field is a struct, recursively collect its nested models
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			nestedModel := reflect.Zero(fieldType).Interface()
			rm.collectNestedModels(nestedModel, allModels, processedModels)
		}

		// If field is a slice/array of structs, process the element type
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			elemType := field.Type.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModel := reflect.Zero(elemType).Interface()
				rm.collectNestedModels(nestedModel, allModels, processedModels)
			}
		}

		// If field is a map with struct values, process the value type
		if field.Type.Kind() == reflect.Map {
			valueType := field.Type.Elem()
			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}
			if valueType.Kind() == reflect.Struct && valueType.String() != "time.Time" {
				nestedModel := reflect.Zero(valueType).Interface()
				rm.collectNestedModels(nestedModel, allModels, processedModels)
			}
		}
	}
}

// generateSchemaFromModelWithCache generates schema using cached models to avoid duplication
func (rm *RouteManager) generateSchemaFromModelWithCache(model interface{}, allModels map[string]interface{}) Schema {
	//// Get the actual type (handle pointers)
	//modelType := reflect.TypeOf(model)
	//if modelType.Kind() == reflect.Ptr {
	//	modelType = modelType.Elem()
	//}
	//
	//// If it's a named struct that exists in allModels, use $ref
	//if modelType.Kind() == reflect.Struct && modelType.String() != "time.Time" {
	//	modelName := modelType.Name()
	//	if modelName != "" {
	//		if _, exists := allModels[modelName]; exists {
	//			return Schema{
	//				Ref: fmt.Sprintf("#/definitions/%s", modelName),
	//			}
	//		}
	//	}
	//}

	// Otherwise, generate the schema normally
	return rm.generateSchemaWithReferences(model)
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

// getModelName extracts the model name from the struct type
func getModelName(model interface{}) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}
