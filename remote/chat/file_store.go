package chat

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultMaxImageSize = 25 * 1024 * 1024 // 25MB
	DefaultMaxDocSize   = 50 * 1024 * 1024 // 50MB
)

// AllowedMIMETypes lists supported file types
var AllowedMIMETypes = map[string]string{
	// Images
	"image/jpeg": "image",
	"image/png":  "image",
	"image/gif":  "image",
	"image/webp": "image",
	// Documents
	"application/pdf": "document",
	"text/plain":      "document",
	"text/markdown":   "document",
	"text/csv":        "document",
	// Word
	"application/msword": "document",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "document",
	// Excel
	"application/vnd.ms-excel": "document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "document",
}

// FileStore handles project-based file storage for bot media
type FileStore struct {
	maxImageSize  int64
	maxDocSize    int64
	httpClient    *http.Client
	telegramToken string // For resolving Telegram file URLs
}

// NewFileStore creates a new file store with default limits
func NewFileStore() *FileStore {
	return &FileStore{
		maxImageSize: DefaultMaxImageSize,
		maxDocSize:   DefaultMaxDocSize,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// NewFileStoreWithProxy creates a new file store with proxy support
func NewFileStoreWithProxy(proxyURL string) (*FileStore, error) {
	httpClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	if proxyURL != "" {
		proxyParsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(proxyParsed),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		}
		httpClient.Transport = transport
	}

	return &FileStore{
		maxImageSize: DefaultMaxImageSize,
		maxDocSize:   DefaultMaxDocSize,
		httpClient:   httpClient,
	}, nil
}

// SetTelegramToken sets the Telegram bot token for resolving file URLs
func (s *FileStore) SetTelegramToken(token string) {
	s.telegramToken = token
}

// StoredFile represents a stored file
type StoredFile struct {
	Path     string // Full path: {projectPath}/.agent/{filename}
	RelPath  string // Relative path for agent: .agent/{filename}
	URL      string // Original URL
	Filename string
	Size     int64
	MimeType string
}

// TelegramFile represents the response from Telegram's getFile API
type TelegramFile struct {
	Ok     bool `json:"ok"`
	Result struct {
		FileID   string `json:"file_id"`
		FileSize int    `json:"file_size"`
		FilePath string `json:"file_path"`
	} `json:"result"`
}

// resolveTelegramFileURL resolves a Telegram file ID to a download URL
func (s *FileStore) resolveTelegramFileURL(ctx context.Context, tgFileURL string) (string, error) {
	if !strings.HasPrefix(tgFileURL, "tgfile://") {
		return tgFileURL, nil
	}

	if s.telegramToken == "" {
		return "", fmt.Errorf("Telegram token not set, cannot resolve file URL")
	}

	fileID := strings.TrimPrefix(tgFileURL, "tgfile://")
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile", s.telegramToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add file_id parameter
	q := req.URL.Query()
	q.Add("file_id", fileID)
	req.URL.RawQuery = q.Encode()

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get file info: status %d", resp.StatusCode)
	}

	var tf TelegramFile
	if err := json.NewDecoder(resp.Body).Decode(&tf); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !tf.Ok || tf.Result.FilePath == "" {
		return "", fmt.Errorf("file not found")
	}

	// Return the full download URL
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", s.telegramToken, tf.Result.FilePath), nil
}

// DownloadFile downloads a file from a URL to the project's .download directory
// Returns an error if file size exceeds limits. name is the name the chat
// message gave the file, if any; see chooseFileName for the rest.
func (s *FileStore) DownloadFile(ctx context.Context, projectPath, url, mimeType, name string) (*StoredFile, error) {
	// Validate MIME type first
	if !s.IsAllowedType(mimeType) {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	// Determine file type for size limits
	fileType := AllowedMIMETypes[mimeType]

	// Resolve Telegram file URLs
	downloadURL := url
	var err error
	if strings.HasPrefix(url, "tgfile://") {
		downloadURL, err = s.resolveTelegramFileURL(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve file URL: %w", err)
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Get content length
	size := resp.ContentLength
	if size > 0 {
		// Check size limits before downloading
		if !s.IsAllowedSize(mimeType, size) {
			maxSize := s.maxImageSize
			if fileType == "document" {
				maxSize = s.maxDocSize
			}
			return nil, fmt.Errorf("file too large: %d bytes (max %d bytes)", size, maxSize)
		}
	}

	// The name depends on the first bytes (an extension for a name that has
	// none), so read them before creating the file.
	head := make([]byte, 512)
	n, err := io.ReadFull(resp.Body, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	head = head[:n]

	downloadDir := s.GetDownloadDir(projectPath)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}
	file, filename, err := createExclusive(downloadDir, chooseFileName(name, resp.Header.Get("Content-Disposition"), mimeType, head, time.Now()))
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	filePath := file.Name()

	written, err := file.Write(head)
	if err == nil {
		var rest int64
		rest, err = io.Copy(file, resp.Body)
		written += int(rest)
	}
	if err != nil {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Verify size after download
	if size <= 0 {
		// If content-length was unknown, check actual size
		if !s.IsAllowedSize(mimeType, int64(written)) {
			os.Remove(filePath) // Clean up on error
			maxSize := s.maxImageSize
			if fileType == "document" {
				maxSize = s.maxDocSize
			}
			return nil, fmt.Errorf("file too large: %d bytes (max %d bytes)", written, maxSize)
		}
	}

	return &StoredFile{
		Path:     filePath,
		RelPath:  filepath.Join(".agent", filename),
		URL:      url,
		Filename: filename,
		Size:     int64(written),
		MimeType: mimeType,
	}, nil
}

// IsAllowedType checks if the mime type is allowed
func (s *FileStore) IsAllowedType(mimeType string) bool {
	_, ok := AllowedMIMETypes[mimeType]
	return ok
}

// IsAllowedSize checks if the size is within limits for the mime type
func (s *FileStore) IsAllowedSize(mimeType string, size int64) bool {
	fileType, ok := AllowedMIMETypes[mimeType]
	if !ok {
		return false
	}

	switch fileType {
	case "image":
		return size <= s.maxImageSize
	case "document":
		return size <= s.maxDocSize
	default:
		return false
	}
}

// GetDownloadDir returns the .download directory for a project
func (s *FileStore) GetDownloadDir(projectPath string) string {
	return filepath.Join(projectPath, ".agent")
}

// createExclusive creates dir/name, adding " (2)", " (3)" ... before the
// extension when the name is taken, so two files sent under one name (or two
// unnamed images in the same second) never overwrite each other.
func createExclusive(dir, name string) (*os.File, string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := os.OpenFile(filepath.Join(dir, candidate), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
			return f, candidate, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("too many files named %q", name)
}
