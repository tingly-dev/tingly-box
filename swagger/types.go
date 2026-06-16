package swagger

// SwaggerInfo holds information for swagger documentation (shared by v2 and v3)
type SwaggerInfo struct {
	Title          string
	Description    string
	Version        string
	Host           string
	BasePath       string
	Schemes        []string
	TermsOfService string
	Contact        SwaggerContact
	License        SwaggerLicense
}

// SwaggerContact holds contact information (shared)
type SwaggerContact struct {
	Name  string
	Email string
	URL   string
}

// SwaggerLicense holds license information (shared)
type SwaggerLicense struct {
	Name string
	URL  string
}

// Schema represents a schema object in Swagger/OpenAPI (shared between v2 and v3)
// Note: This is the common subset that works for both versions
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

// XML represents XML object in Swagger/OpenAPI (shared)
type XML struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Attribute bool   `json:"attribute,omitempty"`
	Wrapped   bool   `json:"wrapped,omitempty"`
}

// ExternalDocs represents external documentation object (shared)
type ExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// Tag represents a tag object in Swagger/OpenAPI (shared)
type Tag struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty"`
}

// Header represents a header object in Swagger/OpenAPI (shared)
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

// Items represents items object for array parameters (shared)
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
