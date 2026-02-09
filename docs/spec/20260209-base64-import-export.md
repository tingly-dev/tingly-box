# Base64 Import/Export Format Spec

**Date:** 2025-02-09
**Status:** Draft
**Version:** 1.0

## Overview

Add a new import/export format option alongside the existing JSONL format. The new format encodes the JSONL data as Base64, enabling easy copy-paste operations for configuration sharing.

## Current State

### Existing JSONL Format
- **Location:** `frontend/src/components/rule-card/utils.ts:98-228`
- **Format:** Line-delimited JSON (JSONL)
- **Structure:**
  ```jsonl
  {"type":"metadata","version":"1.0","exported_at":"..."}
  {"type":"rule",...}
  {"type":"provider",...}
  ```

### Import Implementation
- **Location:** `internal/command/app_manager.go:248-425`
- **Function:** `ImportRuleFromJSONL()`
- **Entry Points:**
  - CLI: `cli/tingly-box/cli.go:113` → `internal/command/import.go`
  - GUI: `gui/wails3/services/tingly_service.go:191-198`

### Gaps Identified
1. No CLI export command (export is frontend-only)
2. No frontend import UI (import is CLI-only)
3. No REST API endpoints for import/export

## Requirements

### Functional Requirements
1. **Base64 Export Format**
   - Encode existing JSONL data to Base64 string
   - Support single-line text for easy copy-paste
   - Add version identifier prefix (e.g., `TGB64:1.0:`)

2. **Base64 Import Format**
   - Detect and parse Base64-encoded export data
   - Maintain backward compatibility with JSONL format
   - Support clipboard paste operations

3. **UI/UX Requirements**
   - Add "Copy to Clipboard" button in export dialog
   - Add "Paste from Clipboard" option in import UI
   - Display validation feedback for invalid Base64 data

### Non-Functional Requirements
- Maintain existing JSONL format as default
- No breaking changes to current import/export APIs
- Follow SOLID principles for clean architecture

## Design

### Data Format

```
TGB64:1.0:<base64_encoded_jsonl>
```

**Components:**
1. **Prefix:** `TGB64` - Format identifier
2. **Version:** `1.0` - Format version
3. **Separator:** `:` - Delimiter
4. **Payload:** Base64-encoded JSONL string

**Example (truncated):**
```
TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIiwiZXhwb3J0ZWRfYXQiOiIyMDI1LTAyLTA5VDEyOjAwOjAwWiJ9Cnl7InR5cGUiOiJydWxlIiwi ...
```

### Module Structure

```
internal/
├── export/
│   ├── format.go          # Format detection interface
│   ├── jsonl.go           # Existing JSONL format
│   ├── base64.go          # New Base64 format
│   └── export.go          # Unified export handler
├── import/
│   ├── format.go          # Format detection interface
│   ├── jsonl.go           # Existing JSONL format
│   ├── base64.go          # New Base64 format
│   └── import.go          # Unified import handler
```

### Core Interfaces

```go
// export/format.go
package export

type Format string

const (
    FormatJSONL Format = "jsonl"
    FormatBase64 Format = "base64"
)

type Exporter interface {
    Export(rule *typ.Rule, providers []*typ.Provider) (string, error)
    Format() Format
}

type ExportResult struct {
    Format  Format
    Content string
}
```

```go
// import/format.go
package import

type Format string

const (
    FormatAuto    Format = "auto"    // Auto-detect
    FormatJSONL   Format = "jsonl"
    FormatBase64  Format = "base64"
)

type Importer interface {
    Import(data string) (*ImportResult, error)
    Format() Format
}

type Detector interface {
    Detect(data string) Format
}
```

### Implementation Plan

#### Phase 1: Core Backend (Go)
1. Create `internal/export/` package
   - Implement format detection
   - Implement Base64 exporter
   - Maintain JSONL exporter

2. Create `internal/import/` package
   - Implement format detection
   - Implement Base64 importer
   - Refactor existing JSONL importer

3. Update `internal/command/app_manager.go`
   - Add `ExportRule(format Format)` method
   - Update `ImportRule()` to support format detection

#### Phase 2: CLI
1. Add export command to `cli/tingly-box/cli.go`
   ```bash
   tingly-box export --request-model <model> --scenario <scenario> [--format jsonl|base64]
   ```

2. Update import command
   ```bash
   tingly-box import [file.jsonl] [--format auto|jsonl|base64]
   ```

#### Phase 3: Frontend
1. Update `frontend/src/components/rule-card/utils.ts`
   - Add `exportRuleAsBase64()` function
   - Add `copyToClipboard()` helper

2. Update export UI
   - Add format selector (JSONL/Base64)
   - Add "Copy to Clipboard" button for Base64 format

3. Add import UI (new feature)
   - Create import dialog component
   - Support file upload and clipboard paste
   - Add format detection

#### Phase 4: API
1. Add REST endpoints to `internal/server/`
   - `POST /api/v1/rules/export`
   - `POST /api/v1/rules/import`

2. Update GUI service `gui/wails3/services/tingly_service.go`
   - Add `ExportRule(format string)` method
   - Add `ImportRuleWithFormat(data, format string)` method

## Testing

### Unit Tests
- `internal/export/base64_test.go`
- `internal/import/base64_test.go`
- Format detection edge cases

### Integration Tests
- CLI export/import roundtrip
- GUI export/import roundtrip
- Cross-format compatibility (export as Base64, import as JSONL)

### Test Cases
1. Valid Base64 export/import
2. Invalid Base64 data handling
3. Empty/null data handling
4. Version mismatch handling
5. Clipboard paste with extra whitespace

## Migration Path

1. **Backward Compatible:** JSONL format remains default
2. **Opt-in:** Base64 format is opt-in via `--format` flag
3. **Deprecation:** None planned for JSONL format

## References

- Existing JSONL export: `frontend/src/components/rule-card/utils.ts:98-228`
- Existing JSONL import: `internal/command/app_manager.go:248-425`
- Data types: `internal/typ/type.go`
