package core

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileURLScheme represents different file URL schemes
type FileURLScheme string

const (
	FileURLSchemeLocal    FileURLScheme = "file"   // file://path/to/file
	FileURLSchemeHTTP     FileURLScheme = "http"   // http://...
	FileURLSchemeHTTPS    FileURLScheme = "https"  // https://...
	FileURLSchemeTelegram FileURLScheme = "tgfile" // tgfile://file_id
)

// MediaURLInfo contains parsed information about a media URL
type MediaURLInfo struct {
	Scheme    FileURLScheme // The URL scheme
	LocalPath string        // Local file path (if file://)
	RemoteURL string        // Remote URL (if http/https)
	FileID    string        // Platform-specific file ID (if tgfile://, etc.)
	Original  string        // Original URL string
}

// ParseMediaURL parses a media URL and returns information about it
func ParseMediaURL(mediaURL string) (*MediaURLInfo, error) {
	if mediaURL == "" {
		return nil, fmt.Errorf("empty media URL")
	}

	info := &MediaURLInfo{Original: mediaURL}

	// Check if it's a file:// URL
	if strings.HasPrefix(mediaURL, "file://") {
		info.Scheme = FileURLSchemeLocal
		// Remove file:// prefix and parse
		u, err := url.Parse(mediaURL)
		if err != nil {
			// Fallback: simple string manipulation
			info.LocalPath = strings.TrimPrefix(mediaURL, "file://")
			// Handle URL-encoded paths
			if unescaped, err := url.PathUnescape(info.LocalPath); err == nil {
				info.LocalPath = unescaped
			}
		} else {
			info.LocalPath = u.Path
			// On Windows, file:// URLs might have a host (e.g., file:///C:/path)
			if u.Host != "" && len(u.Host) == 1 && u.Host[0] >= 'A' && u.Host[0] <= 'Z' {
				// Windows drive letter
				info.LocalPath = u.Host + ":" + u.Path
			}
		}
		return info, nil
	}

	// Check for platform-specific schemes
	if strings.HasPrefix(mediaURL, "tgfile://") {
		info.Scheme = FileURLSchemeTelegram
		info.FileID = strings.TrimPrefix(mediaURL, "tgfile://")
		return info, nil
	}

	// Check for http/https URLs
	if strings.HasPrefix(mediaURL, "http://") || strings.HasPrefix(mediaURL, "https://") {
		info.Scheme = FileURLSchemeHTTPS
		info.RemoteURL = mediaURL
		return info, nil
	}

	// Default: treat as local file path (no scheme)
	info.Scheme = FileURLSchemeLocal
	info.LocalPath = mediaURL
	return info, nil
}

// NormalizeMediaURL normalizes a media URL for platform use.
// For platforms that require local file paths (like Weixin), it converts file:// URLs to local paths.
// Returns the normalized URL that should be used by platform implementations.
func NormalizeMediaURL(mediaURL string) (string, error) {
	info, err := ParseMediaURL(mediaURL)
	if err != nil {
		return "", err
	}

	switch info.Scheme {
	case FileURLSchemeLocal:
		// For file:// URLs, return the local path
		return info.LocalPath, nil
	case FileURLSchemeHTTPS, FileURLSchemeHTTP:
		// For remote URLs, return as-is (platform may download)
		return info.RemoteURL, nil
	case FileURLSchemeTelegram:
		// For platform-specific URLs, return as-is
		return info.Original, nil
	default:
		return info.Original, nil
	}
}

// ValidateLocalFile checks if a local file exists and is accessible.
// Returns the file info if valid, error otherwise.
func ValidateLocalFile(filePath string) (*os.File, error) {
	if filePath == "" {
		return nil, fmt.Errorf("empty file path")
	}

	// Clean the path
	filePath = filepath.Clean(filePath)

	// Check if file exists
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		return nil, fmt.Errorf("cannot access file '%s': %w", filePath, err)
	}

	return file, nil
}

// IsLocalFileURL checks if a URL is a local file URL (file:// or plain path)
func IsLocalFileURL(mediaURL string) bool {
	info, err := ParseMediaURL(mediaURL)
	if err != nil {
		return false
	}
	return info.Scheme == FileURLSchemeLocal
}

// IsRemoteURL checks if a URL is a remote URL (http/https)
func IsRemoteURL(mediaURL string) bool {
	info, err := ParseMediaURL(mediaURL)
	if err != nil {
		return false
	}
	return info.Scheme == FileURLSchemeHTTP || info.Scheme == FileURLSchemeHTTPS
}

// GetLocalFilePath extracts the local file path from a media URL if it's a local file.
// Returns empty string if the URL is not a local file.
func GetLocalFilePath(mediaURL string) string {
	info, err := ParseMediaURL(mediaURL)
	if err != nil {
		return ""
	}
	if info.Scheme == FileURLSchemeLocal {
		return info.LocalPath
	}
	return ""
}

// RequiresDownload checks if a media URL needs to be downloaded before use.
// Returns true for remote URLs, false for local files.
func RequiresDownload(mediaURL string) bool {
	return IsRemoteURL(mediaURL)
}
