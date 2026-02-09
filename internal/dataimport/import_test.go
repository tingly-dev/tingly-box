package dataimport

import (
	"strings"
	"testing"
)

func TestFormatConstants(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   string
	}{
		{"Auto format", FormatAuto, "auto"},
		{"JSONL format", FormatJSONL, "jsonl"},
		{"Base64 format", FormatBase64, "base64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(tt.format); got != tt.want {
				t.Errorf("Format = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase64Prefix(t *testing.T) {
	if Base64Prefix != "TGB64" {
		t.Errorf("Base64Prefix = %v, want TGB64", Base64Prefix)
	}
}

func TestDetectorDetect(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		data   string
		want   Format
	}{
		{
			name: "Base64 format with TGB64 prefix",
			data: "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIn0=",
			want: FormatBase64,
		},
		{
			name: "JSONL format - starts with metadata",
			data: `{"type":"metadata","version":"1.0"}`,
			want: FormatJSONL,
		},
		{
			name: "JSONL format - starts with rule",
			data: `{"type":"rule","uuid":"123"}`,
			want: FormatJSONL,
		},
		{
			name: "Empty string defaults to JSONL",
			data: "",
			want: FormatJSONL,
		},
		{
			name: "Whitespace only defaults to JSONL",
			data: "   \n  \t  ",
			want: FormatJSONL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.data)
			if got != tt.want {
				t.Errorf("Detector.Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase64ImporterDecode(t *testing.T) {
	importer := NewBase64Importer()

	tests := []struct {
		name       string
		data       string
		wantErr    bool
		errMessage string
	}{
		{
			name:    "Valid Base64 export",
			data:    "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIn0KeyJ0eXBlIjoicnVsZSIsInV1aWQiOiJhYmMxMjMifQ==",
			wantErr: false,
		},
		{
			name:       "Missing prefix",
			data:       "invalid:data",
			wantErr:    true,
			errMessage: "missing TGB64 prefix",
		},
		{
			name:       "Invalid format - not enough parts",
			data:       "TGB64:1.0",
			wantErr:    true,
			errMessage: "expected prefix:version:payload",
		},
		{
			name:       "Invalid version",
			data:       "TGB64:2.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIn0=",
			wantErr:    true,
			errMessage: "unsupported version",
		},
		{
			name:       "Invalid Base64",
			data:       "TGB64:1.0:not-valid-base64!@#",
			wantErr:    true,
			errMessage: "failed to decode Base64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := importer.decodeBase64Export(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Base64Importer.decodeBase64Export() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errMessage != "" {
					// Check if error message contains expected text
					if err == nil {
						t.Errorf("Base64Importer.decodeBase64Export() expected error, got nil")
					} else if !strings.Contains(err.Error(), tt.errMessage) {
						t.Errorf("Base64Importer.decodeBase64Export() error = %v, want包含 %v", err.Error(), tt.errMessage)
					}
				}
				return
			}
			if got == "" {
				t.Error("Base64Importer.decodeBase64Export() returned empty string")
			}
			// Check that the decoded content is valid JSONL (has newlines between JSON objects)
			if !strings.Contains(got, "\n") {
				t.Error("Base64Importer.decodeBase64Export() decoded content should contain newlines for JSONL format")
			}
		})
	}
}

func TestNewImporter(t *testing.T) {
	tests := []struct {
		name    string
		format  Format
		wantErr bool
	}{
		{"JSONL importer", FormatJSONL, false},
		{"Base64 importer", FormatBase64, false},
		{"Auto importer (defaults to JSONL)", FormatAuto, false},
		{"Invalid format", Format("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer, err := NewImporter(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewImporter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if importer == nil {
					t.Error("NewImporter() returned nil importer")
				}
			}
		})
	}
}
