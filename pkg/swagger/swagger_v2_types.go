package swagger

// Swagger represents the complete Swagger 2.0 specification
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

// SwaggerInfoObject represents the info object in Swagger 2.0
type SwaggerInfoObject struct {
	Title          string                `json:"title"`
	Description    string                `json:"description,omitempty"`
	TermsOfService string                `json:"termsOfService,omitempty"`
	Contact        *SwaggerContactObject `json:"contact,omitempty"`
	License        *SwaggerLicenseObject `json:"license,omitempty"`
	Version        string                `json:"version"`
}

// SwaggerContactObject represents contact information in Swagger 2.0
type SwaggerContactObject struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// SwaggerLicenseObject represents license information in Swagger 2.0
type SwaggerLicenseObject struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

// PathItem represents a path item object in Swagger 2.0
type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Head    *Operation `json:"head,omitempty"`
}

// Operation represents an operation object in Swagger 2.0
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

// Parameter represents a parameter object in Swagger 2.0
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

// Response represents a response object in Swagger 2.0
type Response struct {
	Description string                 `json:"description"`
	Schema      *Schema                `json:"schema,omitempty"`
	Headers     map[string]Header      `json:"headers,omitempty"`
	Examples    map[string]interface{} `json:"examples,omitempty"`
}

// SecurityScheme represents a security scheme in Swagger 2.0
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
