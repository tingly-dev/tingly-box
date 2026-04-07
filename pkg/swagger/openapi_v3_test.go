package swagger

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGenerateOpenAPIV3(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:       "Test API",
		Description: "Test API Description",
		Version:     "1.0.0",
		Host:        "localhost:8080",
		BasePath:    "/api/v1",
	})

	// Create a test group with routes
	group := rm.NewGroup("test", "v1", "")

	// Test models
	type TestRequest struct {
		Name string `json:"name" binding:"required"`
		Age  int    `json:"age" binding:"gte=0,lte=150"`
	}

	type TestResponse struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	// Register a test route
	group.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "test"})
	},
		WithDescription("Test endpoint"),
		WithQueryModel(TestRequest{}),
		WithResponseModel(TestResponse{}),
	)

	group.POST("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "test"})
	},
		WithDescription("Create test resource"),
		WithRequestModel(TestRequest{}),
		WithResponseModel(TestResponse{}),
	)

	// Generate v3 spec
	v3JSON, err := rm.GenerateOpenAPI(VersionV3)
	assert.NoError(t, err)
	assert.NotEmpty(t, v3JSON)

	// Parse JSON to verify structure
	var openapi OpenAPI
	err = json.Unmarshal([]byte(v3JSON), &openapi)
	assert.NoError(t, err)

	// Verify root structure
	assert.Equal(t, "3.0.3", openapi.OpenAPI)
	assert.Equal(t, "Test API", openapi.Info.Title)
	assert.Equal(t, "1.0.0", openapi.Info.Version)

	// Verify servers
	assert.Len(t, openapi.Servers, 1)
	assert.Equal(t, "https://localhost:8080/api/v1", openapi.Servers[0].URL)

	// Verify paths
	assert.NotEmpty(t, openapi.Paths)
	assert.Contains(t, openapi.Paths, "/test/v1/test")

	// Verify GET operation
	getOp := openapi.Paths["/test/v1/test"].Get
	assert.NotNil(t, getOp)
	assert.Equal(t, "Test endpoint", getOp.Summary)
	assert.NotEmpty(t, getOp.OperationID)
	assert.NotNil(t, getOp.Parameters)

	// Verify POST operation has request body
	postOp := openapi.Paths["/test/v1/test"].Post
	assert.NotNil(t, postOp)
	assert.NotNil(t, postOp.RequestBody)
	assert.True(t, postOp.RequestBody.Required)
	assert.Contains(t, postOp.RequestBody.Content, "application/json")

	// Verify components
	assert.NotNil(t, openapi.Components.Schemas)
	assert.NotEmpty(t, openapi.Components.Schemas)
	assert.Contains(t, openapi.Components.Schemas, "TestRequest")
	assert.Contains(t, openapi.Components.Schemas, "TestResponse")

	// Verify security schemes
	assert.NotNil(t, openapi.Components.SecuritySchemes)
	assert.Contains(t, openapi.Components.SecuritySchemes, "ApiKeyAuth")
	assert.Equal(t, "apiKey", openapi.Components.SecuritySchemes["ApiKeyAuth"].Type)
}

func TestGenerateOpenAPIV2(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:       "Test API",
		Description: "Test API Description",
		Version:     "1.0.0",
		Host:        "localhost:8080",
		BasePath:    "/api/v1",
	})

	// Create a test group with routes
	group := rm.NewGroup("test", "v1", "")

	type TestResponse struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	group.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "test"})
	},
		WithDescription("Test endpoint"),
		WithResponseModel(TestResponse{}),
	)

	// Generate v2 spec (should be backward compatible)
	v2JSON, err := rm.GenerateOpenAPI(VersionV2)
	assert.NoError(t, err)
	assert.NotEmpty(t, v2JSON)

	// Parse JSON to verify structure
	var swagger Swagger
	err = json.Unmarshal([]byte(v2JSON), &swagger)
	assert.NoError(t, err)

	// Verify v2 structure
	assert.Equal(t, "2.0", swagger.Swagger)
	assert.Equal(t, "Test API", swagger.Info.Title)
	assert.Contains(t, swagger.Paths, "/test/v1/test")
	assert.Contains(t, swagger.Definitions, "TestResponse")
}

func TestGetRefPrefix(t *testing.T) {
	assert.Equal(t, "#/definitions/", getRefPrefix(VersionV2))
	assert.Equal(t, "#/components/schemas/", getRefPrefix(VersionV3))
}

func TestOpenAPIV3WithValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:    "Validation Test API",
		Version:  "1.0.0",
		Host:     "localhost:8080",
		BasePath: "/api/v1",
	})

	group := rm.NewGroup("test", "v1", "")

	type ValidatedRequest struct {
		Email    string `json:"email" binding:"required,email"`
		Age      int    `json:"age" binding:"gte=0,lte=150"`
		Password string `json:"password" binding:"min=8"`
		URL      string `json:"url" binding:"url"`
	}

	group.POST("/validated", func(c *gin.Context) {
		c.JSON(200, gin.H{})
	},
		WithDescription("Validated endpoint"),
		WithRequestModel(ValidatedRequest{}),
	)

	v3JSON, err := rm.GenerateOpenAPI(VersionV3)
	assert.NoError(t, err)

	var openapi OpenAPI
	err = json.Unmarshal([]byte(v3JSON), &openapi)
	assert.NoError(t, err)

	// Check validation in schema
	emailSchema := openapi.Components.Schemas["ValidatedRequest"].Properties["email"]
	assert.Equal(t, "email", emailSchema.Format)

	ageSchema := openapi.Components.Schemas["ValidatedRequest"].Properties["age"]
	assert.NotNil(t, ageSchema.Minimum)
	assert.Equal(t, 0.0, *ageSchema.Minimum)
	assert.NotNil(t, ageSchema.Maximum)
	assert.Equal(t, 150.0, *ageSchema.Maximum)

	passwordSchema := openapi.Components.Schemas["ValidatedRequest"].Properties["password"]
	assert.NotNil(t, passwordSchema.MinLength)
	assert.Equal(t, 8, *passwordSchema.MinLength)

	urlSchema := openapi.Components.Schemas["ValidatedRequest"].Properties["url"]
	assert.Equal(t, "uri", urlSchema.Format)
}

func TestOpenAPIV3WithNestedModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	rm := NewRouteManager(engine)
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:    "Nested Models API",
		Version:  "1.0.0",
		Host:     "localhost:8080",
		BasePath: "/api/v1",
	})

	group := rm.NewGroup("test", "v1", "")

	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		Country string `json:"country"`
	}

	type User struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	group.GET("/user", func(c *gin.Context) {
		c.JSON(200, gin.H{})
	},
		WithDescription("Get user with nested address"),
		WithResponseModel(User{}),
	)

	v3JSON, err := rm.GenerateOpenAPI(VersionV3)
	assert.NoError(t, err)

	var openapi OpenAPI
	err = json.Unmarshal([]byte(v3JSON), &openapi)
	assert.NoError(t, err)

	// Both User and Address should be in components
	assert.Contains(t, openapi.Components.Schemas, "User")
	assert.Contains(t, openapi.Components.Schemas, "Address")

	// User should have $ref to Address
	userSchema := openapi.Components.Schemas["User"]
	addressRef := userSchema.Properties["address"].Ref
	assert.Equal(t, "#/components/schemas/Address", addressRef)
}
