package dataio

import (
	"strings"
	"testing"
)

func TestDetectorDetect(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name string
		data string
		want Format
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
			got, err := DecodeBase64Export(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeBase64Export() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("DecodeBase64Export() expected error, got nil")
				} else if !strings.Contains(err.Error(), tt.errMessage) {
					t.Errorf("DecodeBase64Export() error = %v, want contains %v", err.Error(), tt.errMessage)
				}
				return
			}
			if got == "" {
				t.Error("DecodeBase64Export() returned empty string")
			}
			if !strings.Contains(got, "\n") {
				t.Error("decoded content should contain newlines for JSONL format")
			}
		})
	}
}
