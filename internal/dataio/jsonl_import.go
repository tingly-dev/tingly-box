package dataio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ImportOptions controls how imports are handled when conflicts occur
type ImportOptions struct {
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string
	// Quiet suppresses progress output
	Quiet bool
}

// ProviderImportInfo contains information about an imported or used provider
type ProviderImportInfo struct {
	UUID   string
	Name   string
	Action string // "created", "used", "skipped"
}

// ImportResult contains the results of an import operation
type ImportResult struct {
	ProvidersCreated int
	ProvidersUsed    int
	Providers        []ProviderImportInfo
	ProviderMap      map[string]string // old UUID -> new UUID
}

// Importer defines the interface for import implementations
type Importer interface {
	Import(data string, globalConfig *config.Config, opts ImportOptions) (*ImportResult, error)
	Format() Format
}

// JSONLImporter imports data from JSONL format
type JSONLImporter struct{}

// NewJSONLImporter creates a new JSONL importer
func NewJSONLImporter() *JSONLImporter {
	return &JSONLImporter{}
}

// Format returns the format type
func (i *JSONLImporter) Format() Format {
	return FormatJSONL
}

// Import imports data from JSONL format
func (i *JSONLImporter) Import(data string, globalConfig *config.Config, opts ImportOptions) (*ImportResult, error) {
	result := &ImportResult{
		ProviderMap: make(map[string]string),
	}

	// Set defaults
	if opts.OnProviderConflict == "" {
		opts.OnProviderConflict = "use"
	}

	// Parse lines
	scanner := bufio.NewScanner(strings.NewReader(data))
	var metadata *Metadata
	providersData := []*ProviderData{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		// Parse line type
		var base DataLine
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		switch base.Type {
		case "metadata":
			if err := json.Unmarshal([]byte(line), &metadata); err != nil {
				return nil, fmt.Errorf("line %d: invalid metadata: %w", lineNum, err)
			}
			if metadata.Version != "1.0" {
				return nil, fmt.Errorf("unsupported export version: %s", metadata.Version)
			}

		case "provider":
			var provider ProviderData
			if err := json.Unmarshal([]byte(line), &provider); err != nil {
				return nil, fmt.Errorf("line %d: invalid provider data: %w", lineNum, err)
			}
			providersData = append(providersData, &provider)

		default:
			return nil, fmt.Errorf("line %d: unknown type '%s'", lineNum, base.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	if len(providersData) == 0 {
		return nil, fmt.Errorf("no provider data found in export")
	}

	// Import providers
	for _, p := range providersData {
		providerResult, err := i.importProvider(globalConfig, p, opts.OnProviderConflict, result.ProviderMap)
		if err != nil {
			return nil, fmt.Errorf("failed to import provider '%s': %w", p.Name, err)
		}
		if providerResult.created {
			result.ProvidersCreated++
		}
		if providerResult.used {
			result.ProvidersUsed++
		}
		// Add provider info to result
		if providerResult.info != nil {
			result.Providers = append(result.Providers, *providerResult.info)
		}
	}

	return result, nil
}

type providerImportResult struct {
	created bool
	used    bool
	info    *ProviderImportInfo
}

func (i *JSONLImporter) importProvider(globalConfig *config.Config, p *ProviderData, onConflict string, providerMap map[string]string) (*providerImportResult, error) {
	result := &providerImportResult{}

	// Check if provider with same UUID already exists (real conflict)
	existingProvider, err := globalConfig.GetProviderByUUID(p.UUID)
	if err == nil && existingProvider != nil {
		// Real UUID conflict - provider was already imported before
		switch onConflict {
		case "skip":
			result.info = &ProviderImportInfo{
				UUID:   p.UUID,
				Name:   p.Name,
				Action: "skipped",
			}
			return result, nil
		case "use":
			// Use the existing provider
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			result.info = &ProviderImportInfo{
				UUID:   existingProvider.UUID,
				Name:   existingProvider.Name,
				Action: "used",
			}
			return result, nil
		default:
			// Default to using existing provider for UUID conflicts
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			result.info = &ProviderImportInfo{
				UUID:   existingProvider.UUID,
				Name:   existingProvider.Name,
				Action: "used",
			}
			return result, nil
		}
	}

	// Check if provider name already exists (need to avoid duplicate names)
	_, err = globalConfig.GetProviderByName(p.Name)
	nameExists := err == nil

	// Always create a new UUID for imported providers
	// This allows the same provider export to be imported multiple times
	providerUUID := uuid.New().String()

	// If name exists, add suffix to avoid conflicts
	if nameExists {
		suffix := 2
		newName := fmt.Sprintf("%s-%d", p.Name, suffix)
		for {
			_, err := globalConfig.GetProviderByName(newName)
			if err != nil {
				break // Name is available
			}
			suffix++
			newName = fmt.Sprintf("%s-%d", p.Name, suffix)
		}
		p.Name = newName
	}

	// Create new provider with new UUID. Shallow-copy the embedded Provider so
	// we inherit every field automatically, then apply the deliberate
	// import-time overrides below.
	newProvider := p.Provider
	newProvider.UUID = providerUUID
	newProvider.Name = p.Name

	// Imported providers are always user-owned. Without this, an export that
	// happens to carry Source: "builtin" (e.g. from another instance's
	// support bundle) would create a provider that's permanently locked from
	// edit/delete via the API (see Provider.IsBuiltin gating in
	// internal/server/module/provider/handler.go).
	newProvider.Source = typ.ProviderSourceUser

	// LastUpdated is a freshness cache for Models, not portable data; reset it
	// so any staleness-driven refresh logic treats the import as needing a
	// fresh check rather than trusting a timestamp from the source instance.
	newProvider.LastUpdated = ""

	if err := globalConfig.AddProvider(&newProvider); err != nil {
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	// Map old UUID to new UUID
	providerMap[p.UUID] = newProvider.UUID
	result.created = true
	result.info = &ProviderImportInfo{
		UUID:   newProvider.UUID,
		Name:   newProvider.Name,
		Action: "created",
	}
	return result, nil
}
