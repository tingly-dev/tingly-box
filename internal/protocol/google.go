package protocol

import (
	"google.golang.org/genai"
)

type (
	GoogleRequest struct {
		Config  *genai.GenerateContentConfig
		Content []*genai.Content
	}
)
