package swagger

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
