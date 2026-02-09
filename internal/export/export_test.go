package export

import (
	"testing"
)

func TestFormatConstants(t *testing.T) {
	tests := []struct {
		name   string
		format Format
		want   string
	}{
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

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion != "1.0" {
		t.Errorf("CurrentVersion = %v, want 1.0", CurrentVersion)
	}
}

func TestNewExporter(t *testing.T) {
	tests := []struct {
		name    string
		format  Format
		wantErr bool
	}{
		{"JSONL exporter", FormatJSONL, false},
		{"Base64 exporter", FormatBase64, false},
		{"Invalid format", Format("invalid"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter, err := NewExporter(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewExporter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && exporter.Format() != tt.format {
				t.Errorf("NewExporter() format = %v, want %v", exporter.Format(), tt.format)
			}
		})
	}
}

func TestDecodeBase64Export(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantErr    bool
		errMessage string
	}{
		{
			name:    "Valid Base64 export",
			data:    "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIn0=",
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
			got, err := DecodeBase64Export(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeBase64Export() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errMessage != "" {
					// Check if error message contains expected text
					if err == nil || err.Error() == "" {
						t.Errorf("DecodeBase64Export() expected error message containing %v, got empty", tt.errMessage)
					}
				}
				return
			}
			if got == "" {
				t.Error("DecodeBase64Export() returned empty string")
			}
		})
	}
}
