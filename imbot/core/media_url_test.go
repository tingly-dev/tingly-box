package core

import (
	"testing"
)

func TestParseMediaURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantScheme FileURLScheme
		wantLocal  string
		wantRemote string
		wantFileID string
		wantErr    bool
	}{
		{
			name:       "file URL with absolute path",
			url:        "file:///Users/yz/test.txt",
			wantScheme: FileURLSchemeLocal,
			wantLocal:  "/Users/yz/test.txt",
		},
		{
			name:       "file URL without leading slash",
			url:        "file://Users/yz/test.txt",
			wantScheme: FileURLSchemeLocal,
			wantLocal:  "Users/yz/test.txt",
		},
		{
			name:       "file URL with URL encoding",
			url:        "file:///Users/yz/test%20file.txt",
			wantScheme: FileURLSchemeLocal,
			wantLocal:  "/Users/yz/test file.txt",
		},
		{
			name:       "plain local path",
			url:        "/Users/yz/test.txt",
			wantScheme: FileURLSchemeLocal,
			wantLocal:  "/Users/yz/test.txt",
		},
		{
			name:       "relative local path",
			url:        "./test.txt",
			wantScheme: FileURLSchemeLocal,
			wantLocal:  "./test.txt",
		},
		{
			name:       "https URL",
			url:        "https://example.com/file.pdf",
			wantScheme: FileURLSchemeHTTPS,
			wantRemote: "https://example.com/file.pdf",
		},
		{
			name:       "http URL",
			url:        "http://example.com/file.pdf",
			wantScheme: FileURLSchemeHTTP,
			wantRemote: "http://example.com/file.pdf",
		},
		{
			name:       "telegram file URL",
			url:        "tgfile://ABC123",
			wantScheme: FileURLSchemeTelegram,
			wantFileID: "ABC123",
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMediaURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMediaURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Scheme != tt.wantScheme {
				t.Errorf("ParseMediaURL() Scheme = %v, want %v", got.Scheme, tt.wantScheme)
			}
			if got.LocalPath != tt.wantLocal {
				t.Errorf("ParseMediaURL() LocalPath = %v, want %v", got.LocalPath, tt.wantLocal)
			}
			if got.RemoteURL != tt.wantRemote {
				t.Errorf("ParseMediaURL() RemoteURL = %v, want %v", got.RemoteURL, tt.wantRemote)
			}
			if got.FileID != tt.wantFileID {
				t.Errorf("ParseMediaURL() FileID = %v, want %v", got.FileID, tt.wantFileID)
			}
		})
	}
}

func TestNormalizeMediaURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "file URL to local path",
			url:  "file:///Users/yz/test.txt",
			want: "/Users/yz/test.txt",
		},
		{
			name: "file URL without leading slash",
			url:  "file://Users/yz/test.txt",
			want: "Users/yz/test.txt",
		},
		{
			name: "https URL unchanged",
			url:  "https://example.com/file.pdf",
			want: "https://example.com/file.pdf",
		},
		{
			name: "http URL unchanged",
			url:  "http://example.com/file.pdf",
			want: "http://example.com/file.pdf",
		},
		{
			name: "telegram file URL unchanged",
			url:  "tgfile://ABC123",
			want: "tgfile://ABC123",
		},
		{
			name: "plain local path unchanged",
			url:  "/Users/yz/test.txt",
			want: "/Users/yz/test.txt",
		},
		{
			name:    "empty URL error",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeMediaURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeMediaURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got != tt.want {
				t.Errorf("NormalizeMediaURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsLocalFileURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"file URL", "file:///Users/yz/test.txt", true},
		{"plain path", "/Users/yz/test.txt", true},
		{"relative path", "./test.txt", true},
		{"https URL", "https://example.com/file.pdf", false},
		{"http URL", "http://example.com/file.pdf", false},
		{"telegram file", "tgfile://ABC123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLocalFileURL(tt.url); got != tt.want {
				t.Errorf("IsLocalFileURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"https URL", "https://example.com/file.pdf", true},
		{"http URL", "http://example.com/file.pdf", true},
		{"file URL", "file:///Users/yz/test.txt", false},
		{"plain path", "/Users/yz/test.txt", false},
		{"telegram file", "tgfile://ABC123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRemoteURL(tt.url); got != tt.want {
				t.Errorf("IsRemoteURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLocalFilePath(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"file URL", "file:///Users/yz/test.txt", "/Users/yz/test.txt"},
		{"plain path", "/Users/yz/test.txt", "/Users/yz/test.txt"},
		{"https URL", "https://example.com/file.pdf", ""},
		{"telegram file", "tgfile://ABC123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLocalFilePath(tt.url); got != tt.want {
				t.Errorf("GetLocalFilePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiresDownload(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"https URL", "https://example.com/file.pdf", true},
		{"http URL", "http://example.com/file.pdf", true},
		{"file URL", "file:///Users/yz/test.txt", false},
		{"plain path", "/Users/yz/test.txt", false},
		{"telegram file", "tgfile://ABC123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresDownload(tt.url); got != tt.want {
				t.Errorf("RequiresDownload() = %v, want %v", got, tt.want)
			}
		})
	}
}
