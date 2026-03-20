package protocol

// AuxStreamModel is a helper struct for extracting Stream and Model fields
// during JSON unmarshaling of request types that embed SDK types.
// These fields often conflict with embedded SDK type fields, so they
// need to be extracted separately before unmarshaling the inner type.
type AuxStreamModel struct {
	Stream bool   `json:"stream"`
	Model  string `json:"model"`
}
