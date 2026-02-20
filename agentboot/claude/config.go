package claude

// Config holds Claude-specific configuration
type Config struct {
	EnableStreamJSON bool   `json:"enable_stream_json"`
	StreamBufferSize int    `json:"stream_buffer_size"`
	Model            string `json:"model,omitempty"`
}
