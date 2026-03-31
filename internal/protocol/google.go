package protocol

import (
	"google.golang.org/genai"
)

// GoogleRequest wraps Google API request parameters
// Google's SDK uses separate parameters rather than a single request struct
type GoogleRequest struct {
	Model    string
	Contents []*genai.Content
	Config   *genai.GenerateContentConfig
}
