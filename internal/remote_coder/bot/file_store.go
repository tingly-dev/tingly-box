package bot

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
	"application/pdf":                                      "document",
	"text/plain":                                          "document",
	"text/markdown":                                       "document",
	"text/csv":                                            "document",
	// Word
	"application/msword":                                  "document",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "document",
	// Excel
	"application/vnd.ms-excel":                            "document",
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

// NewFileStoreWithLimits creates a new file store with custom limits
func NewFileStoreWithLimits(maxImageSize, maxDocSize int64) *FileStore {
	return &FileStore{
		maxImageSize: maxImageSize,
		maxDocSize:   maxDocSize,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
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
// Returns an error if file size exceeds limits
func (s *FileStore) DownloadFile(ctx context.Context, projectPath, url, mimeType string) (*StoredFile, error) {
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

	// Generate filename
	filename := s.generateFilename(url, mimeType)
	if filename == "" {
		filename = fmt.Sprintf("%d-%s", time.Now().Unix(), "download")
	}

	// Get download directory
	downloadDir := s.GetDownloadDir(projectPath)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Create file
	filePath := filepath.Join(downloadDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download and write file with size tracking
	written, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Verify size after download
	if size <= 0 {
		// If content-length was unknown, check actual size
		if !s.IsAllowedSize(mimeType, written) {
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
		Size:     written,
		MimeType: mimeType,
	}, nil
}

// StoreFile stores a file from a reader to the project's .download directory
func (s *FileStore) StoreFile(ctx context.Context, projectPath string, reader io.Reader, filename, mimeType string) (*StoredFile, error) {
	// Validate MIME type first
	if !s.IsAllowedType(mimeType) {
		return nil, fmt.Errorf("unsupported file type: %s", mimeType)
	}

	// Get download directory
	downloadDir := s.GetDownloadDir(projectPath)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download directory: %w", err)
	}

	// Create file
	filePath := filepath.Join(downloadDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Use limited reader to enforce size limits
	fileType := AllowedMIMETypes[mimeType]
	maxSize := s.maxImageSize
	if fileType == "document" {
		maxSize = s.maxDocSize
	}
	limitedReader := io.LimitReader(reader, maxSize+1)

	written, err := io.Copy(file, limitedReader)
	if err != nil {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Check if we hit the limit
	if written > maxSize {
		os.Remove(filePath) // Clean up on error
		return nil, fmt.Errorf("file too large: exceeds limit of %d bytes", maxSize)
	}

	return &StoredFile{
		Path:     filePath,
		RelPath:  filepath.Join(".agent", filename),
		URL:      "",
		Filename: filename,
		Size:     written,
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

// generateFilename generates a filename from URL and MIME type
func (s *FileStore) generateFilename(url, mimeType string) string {
	// Try to extract filename from URL
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove query parameters
		if idx := strings.Index(filename, "?"); idx > 0 {
			filename = filename[:idx]
		}
		if filename != "" {
			return filename
		}
	}

	// Generate filename based on MIME type
	ext := ""
	switch mimeType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "application/pdf":
		ext = ".pdf"
	case "text/plain":
		ext = ".txt"
	case "text/markdown":
		ext = ".md"
	case "text/csv":
		ext = ".csv"
	// Word
	case "application/msword":
		ext = ".doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		ext = ".docx"
	// Excel
	case "application/vnd.ms-excel":
		ext = ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		ext = ".xlsx"
	}

	return fmt.Sprintf("%d%s", time.Now().Unix(), ext)
}
