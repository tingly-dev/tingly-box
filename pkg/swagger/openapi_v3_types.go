package swagger

// OpenAPI represents the complete OpenAPI 3.0 specification
type OpenAPI struct {
	OpenAPI      string                `json:"openapi"`
	Info         OpenAPIInfoObject     `json:"info"`
	Servers      []Server              `json:"servers,omitempty"`
	Paths        map[string]PathItemV3 `json:"paths"`
	Components   Components            `json:"components,omitempty"`
	Security     []map[string][]string `json:"security,omitempty"`
	Tags         []Tag                 `json:"tags,omitempty"`
	ExternalDocs *ExternalDocs         `json:"externalDocs,omitempty"`
}

// OpenAPIInfoObject represents the info object in OpenAPI 3.0
type OpenAPIInfoObject struct {
	Title          string                `json:"title"`
	Description    string                `json:"description,omitempty"`
	TermsOfService string                `json:"termsOfService,omitempty"`
	Contact        *OpenAPIContactObject `json:"contact,omitempty"`
	License        *OpenAPILicenseObject `json:"license,omitempty"`
	Version        string                `json:"version"`
}

// OpenAPIContactObject represents contact information
type OpenAPIContactObject struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// OpenAPILicenseObject represents license information
type OpenAPILicenseObject struct {
	Name       string `json:"name,omitempty"`
	URL        string `json:"url,omitempty"`
	Identifier string `json:"identifier,omitempty"` // OpenAPI 3.1+
}

// Server represents a server object in OpenAPI 3.0
type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
}

// ServerVariable represents a server variable
type ServerVariable struct {
	Enum        []string `json:"enum,omitempty"`
	Default     string   `json:"default"`
	Description string   `json:"description,omitempty"`
}

// PathItemV3 represents a path item in OpenAPI 3.0
type PathItemV3 struct {
	Ref        string        `json:"$ref,omitempty"`
	Summary    string        `json:"summary,omitempty"`
	Describe   string        `json:"description,omitempty"`
	Get        *OperationV3  `json:"get,omitempty"`
	Post       *OperationV3  `json:"post,omitempty"`
	Put        *OperationV3  `json:"put,omitempty"`
	Delete     *OperationV3  `json:"delete,omitempty"`
	Patch      *OperationV3  `json:"patch,omitempty"`
	Options    *OperationV3  `json:"options,omitempty"`
	Head       *OperationV3  `json:"head,omitempty"`
	Trace      *OperationV3  `json:"trace,omitempty"`
	Servers    []Server      `json:"servers,omitempty"`
	Parameters []ParameterV3 `json:"parameters,omitempty"`
}

// OperationV3 represents an operation in OpenAPI 3.0
type OperationV3 struct {
	Tags         []string              `json:"tags,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	Description  string                `json:"description,omitempty"`
	ExternalDocs *ExternalDocs         `json:"externalDocs,omitempty"`
	OperationID  string                `json:"operationId,omitempty"`
	Parameters   []ParameterV3         `json:"parameters,omitempty"`
	RequestBody  *RequestBody          `json:"requestBody,omitempty"`
	Responses    map[string]ResponseV3 `json:"responses"`
	Callbacks    map[string]Callback   `json:"callbacks,omitempty"`
	Deprecated   bool                  `json:"deprecated,omitempty"`
	Security     []map[string][]string `json:"security,omitempty"`
	Servers      []Server              `json:"servers,omitempty"`
}

// ParameterV3 represents a parameter in OpenAPI 3.0
type ParameterV3 struct {
	Name            string               `json:"name"`
	In              string               `json:"in"` // "query", "header", "path", "cookie"
	Description     string               `json:"description,omitempty"`
	Required        bool                 `json:"required"`
	Deprecated      bool                 `json:"deprecated,omitempty"`
	AllowEmptyValue bool                 `json:"allowEmptyValue,omitempty"`
	Style           string               `json:"style,omitempty"`
	Explode         bool                 `json:"explode,omitempty"`
	AllowReserved   bool                 `json:"allowReserved,omitempty"`
	Schema          *Schema              `json:"schema,omitempty"`
	Example         interface{}          `json:"example,omitempty"`
	Examples        map[string]Example   `json:"examples,omitempty"`
	Content         map[string]MediaType `json:"content,omitempty"`
}

// RequestBody represents a request body in OpenAPI 3.0
type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

// MediaType represents a media type object
type MediaType struct {
	Schema   *Schema             `json:"schema,omitempty"`
	Example  interface{}         `json:"example,omitempty"`
	Examples map[string]Example  `json:"examples,omitempty"`
	Encoding map[string]Encoding `json:"encoding,omitempty"`
}

// ResponseV3 represents a response in OpenAPI 3.0
type ResponseV3 struct {
	Description string               `json:"description"`
	Headers     map[string]Header    `json:"headers,omitempty"`
	Content     map[string]MediaType `json:"content,omitempty"`
	Links       map[string]Link      `json:"links,omitempty"`
}

// Components represents reusable components in OpenAPI 3.0
type Components struct {
	Schemas         map[string]Schema           `json:"schemas,omitempty"`
	Responses       map[string]ResponseV3       `json:"responses,omitempty"`
	Parameters      map[string]ParameterV3      `json:"parameters,omitempty"`
	Examples        map[string]Example          `json:"examples,omitempty"`
	RequestBodies   map[string]RequestBody      `json:"requestBodies,omitempty"`
	Headers         map[string]Header           `json:"headers,omitempty"`
	SecuritySchemes map[string]SecuritySchemeV3 `json:"securitySchemes,omitempty"`
	Links           map[string]Link             `json:"links,omitempty"`
	Callbacks       map[string]Callback         `json:"callbacks,omitempty"`
}

// SecuritySchemeV3 represents a security scheme in OpenAPI 3.0
type SecuritySchemeV3 struct {
	Type             string     `json:"type"` // "apiKey", "http", "oauth2", "openIdConnect"
	Description      string     `json:"description,omitempty"`
	Name             string     `json:"name,omitempty"`
	In               string     `json:"in,omitempty"`               // "query", "header", "cookie"
	Scheme           string     `json:"scheme,omitempty"`           // For "http" type
	BearerFormat     string     `json:"bearerFormat,omitempty"`     // For "http" type: "JWT"
	Flows            OAuthFlows `json:"flows,omitempty"`            // For "oauth2" type
	OpenIdConnectUrl string     `json:"openIdConnectUrl,omitempty"` // For "openIdConnect" type
}

// OAuthFlows represents OAuth flows configuration
type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

// OAuthFlow represents an OAuth flow
type OAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

// Link represents a link object
type Link struct {
	OperationRef string                 `json:"operationRef,omitempty"`
	OperationID  string                 `json:"operationId,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	RequestBody  interface{}            `json:"requestBody,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Server       *Server                `json:"server,omitempty"`
}

// Callback represents a callback object
type Callback map[string]PathItemV3

// Example represents an example object
type Example struct {
	Summary       string      `json:"summary,omitempty"`
	Description   string      `json:"description,omitempty"`
	Value         interface{} `json:"value,omitempty"`
	ExternalValue string      `json:"externalValue,omitempty"`
}

// Encoding represents encoding configuration
type Encoding struct {
	ContentType   string            `json:"contentType,omitempty"`
	Headers       map[string]Header `json:"headers,omitempty"`
	Style         string            `json:"style,omitempty"`
	Explode       bool              `json:"explode,omitempty"`
	AllowReserved bool              `json:"allowReserved,omitempty"`
}
