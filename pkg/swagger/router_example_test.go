package swagger

import (
	"fmt"
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
