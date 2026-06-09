package smart_mem

// =============================================
// smart_mem API view-models
// =============================================

// PersistRequest is the body of POST /api/v1/smart_mem.
// The payload is an arbitrary JSON object that the caller wants to
// persist. The server stores the raw bytes verbatim and returns a
// UUID handle plus a short auto-derived description.
type PersistRequest struct {
	// Payload is the JSON document to persist. Any shape is accepted.
	Payload map[string]interface{} `json:"payload" binding:"required" description:"Arbitrary JSON document to persist"`
}

// PersistResponse is returned by POST /api/v1/smart_mem.
type PersistResponse struct {
	UUID        string `json:"uuid" example:"4f9c5b2a-8d11-4c2c-9f3b-1e7c6d0f0a02"`
	Description string `json:"description" example:"{\"role\":\"user\",\"content\":\"hello\"} ..."`
	SizeBytes   int    `json:"size_bytes" example:"128"`
}

// RetrieveQuery — path parameter only, kept for swagger documentation.
type RetrieveQuery struct {
	UUID string `json:"uuid" form:"uuid" description:"UUID returned by the persist endpoint"`
}
