package dataio

import (
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// DataLine is the base type for all data lines (export/import)
type DataLine struct {
	Type string `json:"type"`
}

// Metadata represents the metadata line (used for both export and import)
type Metadata struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	ExportedAt string `json:"exported_at"`
}

// ProviderData represents the provider data (used for both export and import).
// It embeds typ.Provider directly so every field on the runtime Provider type
// automatically round-trips through export/import with no per-field mapping
// to maintain — adding a field to ai.Provider requires zero changes here.
// Because typ.Provider embeds without its own json tag, its fields promote to
// the same flat JSON level as Type, preserving the wire format
// (e.g. {"type":"provider","uuid":"...","name":"...",...}).
type ProviderData struct {
	Type string `json:"type"`
	typ.Provider
}
