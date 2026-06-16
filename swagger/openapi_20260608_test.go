package swagger

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestOpenAPIValidation tests that generated OpenAPI JSON is valid
func TestOpenAPIValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:       "Test API",
		Description: "Test API Description",
		Version:     "1.0.0",
		Host:        "localhost:8080",
		BasePath:    "/api",
		Schemes:     []string{"http"},
	})

	// Generate OpenAPI JSON
	openapiJSON, err := rm.GenerateOpenAPI(VersionV3)
	if err != nil {
		t.Fatalf("Failed to generate OpenAPI JSON: %v", err)
	}

	// Parse the JSON
	var openapi map[string]interface{}
	if err := json.Unmarshal([]byte(openapiJSON), &openapi); err != nil {
		t.Fatalf("Failed to parse generated OpenAPI JSON: %v", err)
	}

	// Check basic structure
	if openapi["openapi"] == nil {
		t.Error("Missing 'openapi' field")
	}

	// Validate no invalid type names
	validateTypes(t, openapi)

	// Validate no $ref with sibling properties
	validateRefs(t, openapi)
}

// validateTypes checks that there are no invalid type names like "bool"
func validateTypes(t *testing.T, data interface{}) {
	var checkFunc func(interface{}, string)

	checkFunc = func(v interface{}, path string) {
		switch val := v.(type) {
		case map[string]interface{}:
			// Skip security schemes - they have a 'type' field that's not a data type
			if strings.HasSuffix(path, "/components/securitySchemes") || strings.Contains(path, "/components/securitySchemes/") {
				for _, value := range val {
					checkFunc(value, path)
				}
				return
			}

			// Check if this is a schema object with type field
			if typeField, ok := val["type"].(string); ok {
				// Validate against OpenAPI 3.0 data types
				validTypes := map[string]bool{
					"array":   true,
					"boolean": true,
					"integer": true,
					"null":    true,
					"number":  true,
					"object":  true,
					"string":  true,
				}

				if !validTypes[typeField] {
					t.Errorf("Invalid type '%s' at %s (should be 'boolean', 'integer', 'number', 'string', 'array', 'object', or 'null')",
						typeField, path)
				}

				// Specific check for the bug we fixed
				if typeField == "bool" {
					t.Errorf("Found invalid type 'bool' at %s (should be 'boolean')", path)
				}
			}

			for key, value := range val {
				checkFunc(value, path+"/"+key)
			}

		case []interface{}:
			for i, item := range val {
				checkFunc(item, path+"["+string(rune(i))+"]")
			}
		}
	}

	checkFunc(data, "")
}

// validateRefs checks that $ref properties don't have sibling properties
func validateRefs(t *testing.T, data interface{}) {
	var checkFunc func(interface{}, string)

	checkFunc = func(v interface{}, path string) {
		switch val := v.(type) {
		case map[string]interface{}:
			hasRef := false
			var siblings []string

			// Collect all keys
			for key := range val {
				if key == "$ref" {
					hasRef = true
				} else if key != "$ref" && val[key] != nil && !isIgnoredField(key) {
					siblings = append(siblings, key)
				}
			}

			// In OpenAPI 3.0, $ref cannot have sibling properties
			if hasRef && len(siblings) > 0 {
				t.Errorf("At %s: $ref has sibling properties which is not allowed in OpenAPI 3.0: %s",
					path, strings.Join(siblings, ", "))
			}

			// Recursively check nested structures
			for key, value := range val {
				checkFunc(value, path+"/"+key)
			}

		case []interface{}:
			for i, item := range val {
				checkFunc(item, path+"["+string(rune(i))+"]")
			}
		}
	}

	checkFunc(data, "")
}

// isIgnoredField checks if a field should be ignored when checking $ref siblings
// Some fields in our Schema struct might be present but null/empty
func isIgnoredField(key string) bool {
	// These are fields that might be present in Schema struct but should be null/empty for $ref
	ignoredFields := []string{
		// Don't ignore any fields - we want to catch ALL siblings of $ref
	}
	for _, ignored := range ignoredFields {
		if key == ignored {
			return true
		}
	}
	return false
}

// TestOpenAPIWithRealRoutes tests with actual route definitions
func TestOpenAPIWithRealRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:       "Test API",
		Description: "Test API Description",
		Version:     "1.0.0",
		Host:        "localhost:8080",
		BasePath:    "/api",
		Schemes:     []string{"http"},
	})

	// Create a test route group with various parameter types
	group := rm.NewGroup("test", "v1", "/test")

	// Define test models
	type TestRequest struct {
		Name  string `json:"name" description:"Name field"`
		Count int    `json:"count" description:"Count field"`
	}

	type TestResponse struct {
		Success bool   `json:"success" description:"Success status"`
		Message string `json:"message" description:"Response message"`
	}

	// Add a route with query parameter (testing bool type)
	group.POST("/example", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	}, func(config *RouteConfig) {
		config.Description = "Test endpoint"
		config.QueryParams = []QueryParamConfig{
			{
				Name:     "force",
				Type:     "bool", // This should be normalized to "boolean"
				Required: false,
			},
		}
		config.RequestModel = &TestRequest{}
		config.ResponseModel = &TestResponse{}
	})

	// Generate OpenAPI JSON
	openapiJSON, err := rm.GenerateOpenAPI(VersionV3)
	if err != nil {
		t.Fatalf("Failed to generate OpenAPI JSON: %v", err)
	}

	// Parse and validate
	var openapi map[string]interface{}
	if err := json.Unmarshal([]byte(openapiJSON), &openapi); err != nil {
		t.Fatalf("Failed to parse generated OpenAPI JSON: %v", err)
	}

	// Validate no errors
	validateTypes(t, openapi)
	validateRefs(t, openapi)

	// Specific check: verify "bool" type was normalized to "boolean"
	openapiStr := string(openapiJSON)
	if strings.Contains(openapiStr, `"type": "bool"`) {
		t.Error("Found unnormalized type 'bool' (should be 'boolean')")
	}

	// Check that we don't have $ref with description siblings
	if strings.Contains(openapiStr, `"description"`) && strings.Contains(openapiStr, `"$ref"`) {
		// More detailed check would need to parse the structure
		// The validateRefs function above does this properly
	}
}

// TestNormalizeSchemaType tests the normalizeSchemaType function
func TestNormalizeSchemaType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bool", "boolean"},
		{"int", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"float", "number"},
		{"float32", "number"},
		{"float64", "number"},
		{"string", "string"},
		{"array", "array"},
		{"object", "object"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeSchemaType(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeSchemaType(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestRefSchemaHandling tests that $ref schemas don't get sibling properties
func TestRefSchemaHandling(t *testing.T) {
	// Create a simple test model
	type NestedModel struct {
		Value string `json:"value"`
	}

	type TestModel struct {
		Nested *NestedModel `json:"nested"`
	}

	rm := &RouteManager{}
	version := VersionV3

	// Generate schema for the model
	schema := rm.generateSchemaWithReferencesVersion(TestModel{}, version)

	// Check that the nested property uses $ref
	nestedSchema, exists := schema.Properties["nested"]
	if !exists {
		t.Fatal("nested property not found")
	}

	if nestedSchema.Ref == "" {
		t.Error("nested schema should have a $ref, but Ref is empty")
	}

	// Check that $ref schemas don't have sibling properties
	if nestedSchema.Ref != "" {
		if nestedSchema.Description != "" {
			t.Errorf("$ref schema should not have description, got: %s", nestedSchema.Description)
		}
		if nestedSchema.Type != "" {
			t.Errorf("$ref schema should not have type, got: %s", nestedSchema.Type)
		}
		if nestedSchema.Example != nil {
			t.Errorf("$ref schema should not have example, got: %v", nestedSchema.Example)
		}
	}
}

// TestMarshalSchemaWithRef tests JSON marshaling of schemas with $ref
func TestMarshalSchemaWithRef(t *testing.T) {
	schema := Schema{
		Ref: "#/components/schemas/TestModel",
	}

	// If we add other fields, they should still be marshaled
	// but in valid OpenAPI, $ref should be alone
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Failed to marshal schema: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Check that $ref is present
	if _, ok := result["$ref"]; !ok {
		t.Error("$ref not found in marshaled JSON")
	}

	// In valid OpenAPI 3.0, $ref should be the only property
	// but our Schema struct allows other fields
	// The fix ensures we don't SET other fields when creating $ref schemas
}

// TestBooleanTypeMapping tests that bool types are mapped to boolean
func TestBooleanTypeMapping(t *testing.T) {
	rm := &RouteManager{}

	// Test direct bool type
	boolType := reflect.TypeOf(false)
	schema := rm.getSwaggerType(boolType)

	if schema.Type != "boolean" {
		t.Errorf("Expected type 'boolean' for bool, got '%s'", schema.Type)
	}
}

// TestGenerateSchemaWithRefNoSiblings tests comprehensive schema generation
func TestGenerateSchemaWithRefNoSiblings(t *testing.T) {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city" description:"City name"`
	}

	type User struct {
		Name    string   `json:"name" description:"User name"`
		Address *Address `json:"address" description:"User address"`
		Active  bool     `json:"active" description:"Is user active"`
	}

	rm := &RouteManager{}
	version := VersionV3

	schema := rm.generateSchemaWithReferencesVersion(User{}, version)

	// Check address property (should be $ref)
	addrSchema, exists := schema.Properties["address"]
	if !exists {
		t.Fatal("address property not found")
	}

	if addrSchema.Ref == "" {
		t.Error("address should have $ref")
	}

	// Verify no sibling properties on $ref
	if addrSchema.Ref != "" && addrSchema.Description != "" {
		t.Error("$ref schema should not have description as sibling")
	}

	// Check active property (should be boolean type)
	activeSchema, exists := schema.Properties["active"]
	if !exists {
		t.Fatal("active property not found")
	}

	if activeSchema.Type != "boolean" {
		t.Errorf("Expected type 'boolean' for active, got '%s'", activeSchema.Type)
	}
}

// Helper function to find $ref usage issues in generated JSON
func findRefSiblings(data []byte) []string {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}

	var issues []string
	var checkFunc func(interface{})

	checkFunc = func(v interface{}) {
		switch val := v.(type) {
		case map[string]interface{}:
			hasRef := false
			var otherKeys []string

			for key, value := range val {
				if key == "$ref" {
					hasRef = true
				} else if key != "$ref" && value != nil {
					otherKeys = append(otherKeys, key)
				}
			}

			if hasRef && len(otherKeys) > 0 {
				issues = append(issues, strings.Join(otherKeys, ", "))
			}

			for _, value := range val {
				checkFunc(value)
			}
		case []interface{}:
			for _, item := range val {
				checkFunc(item)
			}
		}
	}

	checkFunc(result)
	return issues
}
