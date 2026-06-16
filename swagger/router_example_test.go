package swagger

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func ExampleRouteManager() {
	// Create gin engine
	engine := gin.New()

	// Create route manager
	manager := NewRouteManager(engine)

	// Set Swagger information
	manager.SetSwaggerInfo(SwaggerInfo{
		Title:       "Group Routes API",
		Description: "A RESTful API built with Go Gin framework featuring route management, user operations, and Swagger documentation generation.",
		Version:     "1.0.0",
		Host:        "localhost:15000",
		BasePath:    "/",
		Contact: SwaggerContact{
			Name:  "API Support",
			Email: "support@example.com",
			URL:   "https://example.com/support",
		},
		License: SwaggerLicense{
			Name: "MIT",
			URL:  "https://opensource.org/licenses/MIT",
		},
	})

	// Create v1 group
	v1 := manager.NewGroup("api", "v1", "")

	// Register ping routes
	v1.GET("/ping", PingHandler,
		WithDescription("Ping endpoint for connectivity check"),
		WithTags("ping"),
		WithResponseModel(struct {
			Message string `json:"message"`
		}{}),
	)

	// Register user routes
	{
		// Create user
		v1.POST("/users", CreateUserHandler,
			WithDescription("Create a new user"),
			WithTags("users"),
			WithRequestModel(CreateUserRequest{}),
			WithResponseModel(User{}),
			WithErrorResponses(
				ErrorResponseConfig{
					Code:    400,
					Message: "Bad Request - Invalid input data",
					Model:   ErrorResponse{},
				},
				ErrorResponseConfig{
					Code:    409,
					Message: "Conflict - User already exists",
					Model:   ErrorResponse{},
				},
			),
		)
	}
}

// ErrorResponse is the standard error response structure
type ErrorResponse struct {
	Success   bool              `json:"success" example:"false"`
	Error     string            `json:"error" example:"Validation failed"`
	Details   map[string]string `json:"details,omitempty"`
	Code      string            `json:"code,omitempty"`
	Timestamp int64             `json:"timestamp" example:"1704067200"`
	RequestID string            `json:"request_id,omitempty"`
}

// Handle implements the Handler interface
func PingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

// User represents a user in the system
type User struct {
	ID        string    `json:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Username  string    `json:"username" example:"john_doe"`
	Email     string    `json:"email" example:"john.doe@example.com"`
	FirstName string    `json:"first_name" example:"John"`
	LastName  string    `json:"last_name" example:"Doe"`
	Avatar    string    `json:"avatar" example:"https://example.com/avatars/john.jpg"`
	Active    bool      `json:"active" example:"true"`
	CreatedAt time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt time.Time `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}

// CreateUserRequest represents the request to create a new user
type CreateUserRequest struct {
	Username  string `json:"username" binding:"required,min=3,max=50,alphanum" description:"Unique username (alphanumeric only)" example:"john_doe"`
	Email     string `json:"email" binding:"required,email" description:"Valid email address" example:"john.doe@example.com"`
	FirstName string `json:"first_name" binding:"required,min=1,max=50" description:"First name" default:"John" example:"John"`
	LastName  string `json:"last_name" binding:"required,min=1,max=50" description:"Last name" default:"Doe" example:"Doe"`
	Password  string `json:"password" binding:"required,min=8,contains=!@#$%^&*" description:"Password with at least 8 characters and special characters" example:"SecurePass123!"`
	Age       *int   `json:"age,omitempty" binding:"omitempty,gte=18,lte=120" description:"User age (optional)" default:"25"`
	UserType  string `json:"user_type" binding:"required,oneof=admin user moderator guest" description:"User role" default:"user" example:"user"`
}

// Handle implements the Handler interface
func CreateUserHandler(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, ErrorResponse{
			Success: false,
			Error:   "Invalid request data",
			Details: map[string]string{
				"validation_error": err.Error(),
			},
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Mock user creation
	user := User{
		ID:        fmt.Sprintf("%d", time.Now().Unix()),
		Username:  req.Username,
		Email:     req.Email,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Active:    true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    user,
		"message": "User created successfully",
	})
}

// Example test for nested models in Swagger generation
func TestRouteManager_nestedModels(t *testing.T) {
	// Create gin engine
	engine := gin.New()

	// Create route manager
	manager := NewRouteManager(engine)

	// Set Swagger information
	manager.SetSwaggerInfo(SwaggerInfo{
		Title:       "Nested Models API",
		Description: "API demonstrating nested model handling in Swagger",
		Version:     "1.0.0",
		Host:        "localhost:15000",
		BasePath:    "/",
	})

	// Create v1 group
	v1 := manager.NewGroup("api", "v1", "")

	// Register a route with nested models
	v1.POST("/companies", CreateCompanyHandler,
		WithDescription("Create a new company with nested address and contacts"),
		WithTags("companies"),
		WithRequestModel(CreateCompanyRequest{}),
		WithResponseModel(CompanyResponse{}),
	)

	swaggerJSON, _ := manager.GenerateSwaggerJSON()
	fmt.Printf("%s\n", swaggerJSON)
}

// Address represents a nested address struct
type Address struct {
	Street  string `json:"street" binding:"required" description:"Street address"`
	City    string `json:"city" binding:"required" description:"City name"`
	Country string `json:"country" binding:"required" description:"Country name"`
	ZipCode string `json:"zip_code" description:"Postal/ZIP code"`
}

// Contact represents a nested contact struct
type Contact struct {
	Name    string  `json:"name" binding:"required" description:"Contact name"`
	Email   string  `json:"email" binding:"required,email" description:"Contact email"`
	Phone   string  `json:"phone" description:"Contact phone"`
	Address Address `json:"address" description:"Contact address"`
}

// CreateCompanyRequest demonstrates nested and array models
type CreateCompanyRequest struct {
	Name        string    `json:"name" binding:"required" description:"Company name"`
	Description string    `json:"description" description:"Company description"`
	Website     string    `json:"website" description:"Company website"`
	Address     Address   `json:"address" binding:"required" description:"Company headquarters"`
	Contacts    []Contact `json:"contacts" description:"List of company contacts"`
	Founded     time.Time `json:"founded" description:"Company founding date"`
	Active      bool      `json:"active" description:"Whether company is active"`
}

// CompanyResponse demonstrates multiple levels of nesting
type CompanyResponse struct {
	ID          string    `json:"id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Name        string    `json:"name" example:"Acme Corp"`
	Description string    `json:"description,omitempty" example:"A leading technology company"`
	Website     string    `json:"website,omitempty" example:"https://acme.com"`
	Address     Address   `json:"address"`
	Contacts    []Contact `json:"contacts"`
	Founded     time.Time `json:"founded" example:"2020-01-01T00:00:00Z"`
	CreatedAt   time.Time `json:"created_at" example:"2024-01-01T00:00:00Z"`
	UpdatedAt   time.Time `json:"updated_at" example:"2024-01-01T00:00:00Z"`
}

// CreateCompanyHandler handles company creation
func CreateCompanyHandler(c *gin.Context) {
	var req CreateCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, ErrorResponse{
			Success:   false,
			Error:     "Invalid request data",
			Details:   map[string]string{"validation_error": err.Error()},
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Mock company creation
	company := CompanyResponse{
		ID:          fmt.Sprintf("%d", time.Now().Unix()),
		Name:        req.Name,
		Description: req.Description,
		Website:     req.Website,
		Address:     req.Address,
		Contacts:    req.Contacts,
		Founded:     req.Founded,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    company,
		"message": "Company created successfully",
	})
}

// Test pointer to nested struct
type CompanyWithPointers struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Address  *Address   `json:"address"`         // Pointer to struct
	Contacts []*Contact `json:"contacts"`        // Slice of pointers to structs
	Owner    *Person    `json:"owner,omitempty"` // Optional pointer to struct
}

type Person struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
}

// Test nested models with pointers
func TestRouteManager_nestedModelsWithPointers(t *testing.T) {
	// Create gin engine
	engine := gin.New()

	// Create route manager
	manager := NewRouteManager(engine)

	// Set Swagger information
	manager.SetSwaggerInfo(SwaggerInfo{
		Title:       "Nested Models with Pointers API",
		Description: "API demonstrating nested model with pointers handling",
		Version:     "1.0.0",
		Host:        "localhost:15000",
		BasePath:    "/",
	})

	// Create v1 group
	v1 := manager.NewGroup("api", "v1", "")

	// Register a route with nested models using pointers
	v1.POST("/companies-pointer", func(c *gin.Context) {
		var req CompanyWithPointers
		c.JSON(200, req)
	},
		WithDescription("Create company with pointer fields"),
		WithTags("companies"),
		WithRequestModel(CompanyWithPointers{}),
		WithResponseModel(CompanyWithPointers{}),
	)

	swaggerJSON, err := manager.GenerateSwaggerJSON()
	if err != nil {
		t.Errorf("Failed to generate Swagger JSON: %v", err)
	}

	// Print the JSON for manual verification
	fmt.Printf("\n=== Swagger JSON with Pointer Types ===\n")
	fmt.Printf("%s\n", swaggerJSON)

	// Check that definitions for Address, Contact, and Person exist
	if !strings.Contains(swaggerJSON, `"definitions"`) {
		t.Error("Definitions section not found in Swagger JSON")
	}

	// Check that $ref references are created for nested structs
	if !strings.Contains(swaggerJSON, `"#/definitions/Address"`) {
		t.Error("$ref to Address definition not found")
	}
	if !strings.Contains(swaggerJSON, `"#/definitions/Contact"`) {
		t.Error("$ref to Contact definition not found")
	}
	if !strings.Contains(swaggerJSON, `"#/definitions/Person"`) {
		t.Error("$ref to Person definition not found")
	}

	// Parse JSON to check structure
	var swaggerData map[string]interface{}
	if err := json.Unmarshal([]byte(swaggerJSON), &swaggerData); err != nil {
		t.Errorf("Failed to parse generated Swagger JSON: %v", err)
	}

	// Check definitions exist
	definitions, ok := swaggerData["definitions"].(map[string]interface{})
	if !ok {
		t.Error("Definitions section is missing or not a map")
		return
	}

	// Verify all required definitions exist
	requiredDefs := []string{"CompanyWithPointers", "Address", "Contact", "Person"}
	for _, defName := range requiredDefs {
		if _, exists := definitions[defName]; !exists {
			t.Errorf("Definition %s not found in definitions", defName)
		}
	}

	// Verify Address definition has correct properties
	if addressDef, exists := definitions["Address"].(map[string]interface{}); exists {
		if props, ok := addressDef["properties"].(map[string]interface{}); ok {
			streets := []string{"street", "city", "country", "zip_code"}
			for _, field := range streets {
				if _, exists := props[field]; !exists {
					t.Errorf("Field %s not found in Address definition", field)
				}
			}
		}
	}
}

// Test anonymous struct naming with explicit name and auto-generation
func TestRouteManager_anonymousStructNaming(t *testing.T) {
	// Create gin engine
	engine := gin.New()

	// Create route manager
	manager := NewRouteManager(engine)

	// Set Swagger information
	manager.SetSwaggerInfo(SwaggerInfo{
		Title:       "Anonymous Struct API",
		Description: "API demonstrating anonymous struct naming",
		Version:     "1.0.0",
		Host:        "localhost:15000",
		BasePath:    "/",
	})

	// Create v1 group
	v1 := manager.NewGroup("api", "v1", "")

	// Test 1: Anonymous struct without explicit name (should auto-generate)
	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	},
		WithDescription("Ping endpoint"),
		WithTags("health"),
		WithResponseModel(struct {
			Message string `json:"message"`
		}{}),
	)

	// Test 2: Anonymous struct with explicit name (using extended API)
	v1.POST("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"success": true})
	},
		WithDescription("Create user"),
		WithTags("users"),
		WithRequestModel(struct {
			Username string `json:"username" binding:"required"`
			Email    string `json:"email" binding:"required,email"`
		}{}, "CreateUserRequest"),
		WithResponseModel(struct {
			ID      string `json:"id"`
			Success bool   `json:"success"`
		}{}, "CreateUserResponse"),
	)

	// Test 3: Named struct (should use struct name as before)
	v1.GET("/companies", func(c *gin.Context) {
		c.JSON(200, []CompanyResponse{})
	},
		WithDescription("List companies"),
		WithTags("companies"),
		WithResponseModel(CompanyResponse{}),
	)

	swaggerJSON, err := manager.GenerateSwaggerJSON()
	if err != nil {
		t.Errorf("Failed to generate Swagger JSON: %v", err)
		return
	}

	// Print the JSON for manual verification
	fmt.Printf("\n=== Swagger JSON with Anonymous Struct Naming ===\n")
	fmt.Printf("%s\n", swaggerJSON)

	// Parse JSON to verify structure
	var swaggerData map[string]interface{}
	if err := json.Unmarshal([]byte(swaggerJSON), &swaggerData); err != nil {
		t.Errorf("Failed to parse generated Swagger JSON: %v", err)
		return
	}

	definitions, ok := swaggerData["definitions"].(map[string]interface{})
	if !ok {
		t.Error("Definitions section is missing or not a map")
		return
	}

	// Test 1: Verify auto-generated name for anonymous struct
	// GET /api/v1/ping -> GetApiV1PingResponse
	autoGenName := "GetApiV1PingResponse"
	if _, exists := definitions[autoGenName]; !exists {
		t.Errorf("Auto-generated definition %s not found. Available: %v", autoGenName, getMapKeys(definitions))
	}

	// Test 2: Verify explicit name is used
	if _, exists := definitions["CreateUserRequest"]; !exists {
		t.Error("Explicit name 'CreateUserRequest' not found in definitions")
	}
	if _, exists := definitions["CreateUserResponse"]; !exists {
		t.Error("Explicit name 'CreateUserResponse' not found in definitions")
	}

	// Test 3: Verify named struct still works
	if _, exists := definitions["CompanyResponse"]; !exists {
		t.Error("Named struct 'CompanyResponse' not found in definitions")
	}

	// Verify references are correct
	paths, ok := swaggerData["paths"].(map[string]interface{})
	if !ok {
		t.Error("Paths section is missing")
		return
	}

	// Check /api/v1/ping response reference
	pingPath, ok := paths["/api/v1/ping"].(map[string]interface{})
	if !ok {
		t.Error("Ping path not found")
		return
	}
	pingGet, ok := pingPath["get"].(map[string]interface{})
	if !ok {
		t.Error("Ping GET method not found")
		return
	}
	pingResponses, ok := pingGet["responses"].(map[string]interface{})
	if !ok {
		t.Error("Ping responses not found")
		return
	}
	ping200, ok := pingResponses["200"].(map[string]interface{})
	if !ok {
		t.Error("Ping 200 response not found")
		return
	}
	pingSchema, ok := ping200["schema"].(map[string]interface{})
	if !ok {
		t.Error("Ping schema not found")
		return
	}
	ref, _ := pingSchema["$ref"].(string)
	expectedRef := "#/definitions/GetApiV1PingResponse"
	if ref != expectedRef {
		t.Errorf("Ping response $ref = %s, want %s", ref, expectedRef)
	}
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
