package onboarding

// TokenCandidate is a possible API token found in the input. The extractor
// stays vendor-agnostic — it just reports what it saw and where it came
// from. The user picks which one (if any) to use.
type TokenCandidate struct {
	Value   string `json:"value"`
	Preview string `json:"preview"`
	Source  string `json:"source"` // bearer | x-api-key | env:NAME | json:api_key | key_prefix
}

// ExtractRequest is the body for POST /api/v1/onboarding/extract.
type ExtractRequest struct {
	Input string `json:"input"`
}

// ExtractData is the inner payload returned by the extractor. It is a flat
// list of detected URLs and tokens — provider matching, if any, is done on
// the client side after the user picks values.
type ExtractData struct {
	URLs   []string         `json:"urls"`
	Tokens []TokenCandidate `json:"tokens"`
}

// ExtractResponse mirrors the rest of the v1 envelope shape used elsewhere.
type ExtractResponse struct {
	Success bool         `json:"success"`
	Data    *ExtractData `json:"data,omitempty"`
	Error   *ErrorDetail `json:"error,omitempty"`
}

// ErrorDetail is a minimal error envelope. Kept local to the module so the
// onboarding API does not depend on internal server types.
type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
}
