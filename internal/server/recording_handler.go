package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RecordingFileInfo represents information about a recording file
type RecordingFileInfo struct {
	Scenario  string    `json:"scenario"`
	Provider  string    `json:"provider"`
	Date      string    `json:"date"`       // YYYY-MM-DD
	Hour      string    `json:"hour"`       // HH
	Path      string    `json:"path"`       // Full relative path from base dir
	Size      int64     `json:"size"`       // File size in bytes
	Count     int       `json:"count"`      // Number of records (estimated)
	CreatedAt time.Time `json:"created_at"` // File creation time
}

// RecordingListResponse represents the response for listing recordings
type RecordingListResponse struct {
	Success bool                `json:"success"`
	Data    []RecordingFileInfo `json:"data"`
}

// RecordingDetailResponse represents the response for recording details
type RecordingDetailResponse struct {
	Success bool               `json:"success"`
	Data    *RecordingFileInfo `json:"data"`
}

// RecordEntry represents a single recording entry from JSONL file
type RecordEntry struct {
	Timestamp  string                 `json:"timestamp"`
	RequestID  string                 `json:"request_id"`
	Provider   string                 `json:"provider"`
	Scenario   string                 `json:"scenario,omitempty"`
	Model      string                 `json:"model"`
	Request    *RecordRequest         `json:"request,omitempty"`
	Response   *RecordResponse        `json:"response"`
	DurationMs int64                  `json:"duration_ms"`
	Error      string                 `json:"error,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// RecordRequest represents the HTTP request details
type RecordRequest struct {
	Method  string                 `json:"method"`
	URL     string                 `json:"url"`
	Headers map[string]string      `json:"headers"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

// RecordResponse represents the HTTP response details
type RecordResponse struct {
	StatusCode   int                    `json:"status_code"`
	Headers      map[string]string      `json:"headers"`
	Body         map[string]interface{} `json:"body,omitempty"`
	IsStreaming  bool                   `json:"is_streaming,omitempty"`
	StreamChunks []string               `json:"-"` // Parsed SSE data chunks
}

// RecordingEntriesResponse represents the response for recording entries
type RecordingEntriesResponse struct {
	Success bool          `json:"success"`
	Data    []RecordEntry `json:"data"`
	Total   int           `json:"total"`
}

// ListRecordings lists all recording files with optional filtering
// Query params:
//   - scenario: filter by scenario
//   - provider: filter by provider
//   - date: filter by date (YYYY-MM-DD)
func (s *Server) ListRecordings(c *gin.Context) {
	if s.recordDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Recording is not configured",
		})
		return
	}

	scenario := c.Query("scenario")
	provider := c.Query("provider")
	date := c.Query("date")

	recordings, err := s.scanRecordingFiles(scenario, provider, date)
	if err != nil {
		logrus.Errorf("Failed to scan recording files: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to scan recordings",
		})
		return
	}

	// Sort by date and hour (newest first)
	slices.SortFunc(recordings, func(a, b RecordingFileInfo) int {
		if a.Date != b.Date {
			return strings.Compare(b.Date, a.Date) // Descending date
		}
		return strings.Compare(b.Hour, a.Hour) // Descending hour
	})

	c.JSON(http.StatusOK, RecordingListResponse{
		Success: true,
		Data:    recordings,
	})
}

// GetRecordingDetails gets details about a specific recording file
// URL params: scenario, provider, date, hour
func (s *Server) GetRecordingDetails(c *gin.Context) {
	if s.recordDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Recording is not configured",
		})
		return
	}

	scenario := c.Param("scenario")
	provider := c.Param("provider")
	date := c.Param("date")
	hour := c.Param("hour")

	if scenario == "" || provider == "" || date == "" || hour == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Missing required parameters: scenario, provider, date, hour",
		})
		return
	}

	// Build file path
	relPath := filepath.Join(scenario, provider, date, hour+".jsonl")
	fullPath := filepath.Join(s.recordDir, relPath)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Recording file not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to access recording file",
			})
		}
		return
	}

	// Count lines in file
	count, err := s.countLinesInFile(fullPath)
	if err != nil {
		logrus.Warnf("Failed to count lines in recording file: %v", err)
		count = -1
	}

	fileInfo := &RecordingFileInfo{
		Scenario:  scenario,
		Provider:  provider,
		Date:      date,
		Hour:      hour,
		Path:      relPath,
		Size:      info.Size(),
		Count:     count,
		CreatedAt: info.ModTime(),
	}

	c.JSON(http.StatusOK, RecordingDetailResponse{
		Success: true,
		Data:    fileInfo,
	})
}

// GetRecordingEntries reads entries from a specific recording file
// URL params: scenario, provider, date, hour
// Query params:
//   - limit: maximum number of entries to return (default: 100)
//   - offset: offset for pagination (default: 0)
func (s *Server) GetRecordingEntries(c *gin.Context) {
	if s.recordDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Recording is not configured",
		})
		return
	}

	scenario := c.Param("scenario")
	provider := c.Param("provider")
	date := c.Param("date")
	hour := c.Param("hour")

	limitStr := c.DefaultQuery("limit", "100")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 100
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// Maximum limit to prevent memory issues
	if limit > 10000 {
		limit = 10000
	}

	// Build file path
	relPath := filepath.Join(scenario, provider, date, hour+".jsonl")
	fullPath := filepath.Join(s.recordDir, relPath)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Recording file not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to open recording file",
			})
		}
		return
	}
	defer file.Close()

	// Read and parse entries
	var entries []RecordEntry
	decoder := json.NewDecoder(file)
	currentLine := 0
	totalSkipped := 0

	for {
		var entry RecordEntry
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed lines
			currentLine++
			continue
		}

		if currentLine < offset {
			currentLine++
			totalSkipped++
			continue
		}

		entries = append(entries, entry)
		currentLine++

		if len(entries) >= limit {
			break
		}
	}

	c.JSON(http.StatusOK, RecordingEntriesResponse{
		Success: true,
		Data:    entries,
		Total:   currentLine + totalSkipped,
	})
}

// DeleteRecording deletes a specific recording file
// URL params: scenario, provider, date, hour
func (s *Server) DeleteRecording(c *gin.Context) {
	if s.recordDir == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Recording is not configured",
		})
		return
	}

	scenario := c.Param("scenario")
	provider := c.Param("provider")
	date := c.Param("date")
	hour := c.Param("hour")

	if scenario == "" || provider == "" || date == "" || hour == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Missing required parameters: scenario, provider, date, hour",
		})
		return
	}

	// Build file path
	relPath := filepath.Join(scenario, provider, date, hour+".jsonl")
	fullPath := filepath.Join(s.recordDir, relPath)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Recording file not found",
			})
		} else {
			logrus.Errorf("Failed to delete recording file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to delete recording file",
			})
		}
		return
	}

	// Try to clean up empty directories
	s.cleanupEmptyDirs(filepath.Join(s.recordDir, scenario, provider, date))
	s.cleanupEmptyDirs(filepath.Join(s.recordDir, scenario, provider))
	s.cleanupEmptyDirs(filepath.Join(s.recordDir, scenario))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Recording deleted successfully",
	})
}

// scanRecordingFiles scans the recording directory for files matching filters
func (s *Server) scanRecordingFiles(scenarioFilter, providerFilter, dateFilter string) ([]RecordingFileInfo, error) {
	var recordings []RecordingFileInfo

	// Walk through scenario directories
	scenarioDirs, err := os.ReadDir(s.recordDir)
	if err != nil {
		if os.IsNotExist(err) {
			return recordings, nil // No recordings yet
		}
		return nil, err
	}

	for _, scenarioEntry := range scenarioDirs {
		if !scenarioEntry.IsDir() {
			continue
		}
		scenario := scenarioEntry.Name()

		// Filter by scenario
		if scenarioFilter != "" && scenario != scenarioFilter {
			continue
		}

		scenarioPath := filepath.Join(s.recordDir, scenario)
		providerDirs, err := os.ReadDir(scenarioPath)
		if err != nil {
			logrus.Warnf("Failed to read provider directory %s: %v", scenarioPath, err)
			continue
		}

		for _, providerEntry := range providerDirs {
			if !providerEntry.IsDir() {
				continue
			}
			provider := providerEntry.Name()

			// Filter by provider
			if providerFilter != "" && provider != providerFilter {
				continue
			}

			providerPath := filepath.Join(scenarioPath, provider)
			dateDirs, err := os.ReadDir(providerPath)
			if err != nil {
				logrus.Warnf("Failed to read date directory %s: %v", providerPath, err)
				continue
			}

			for _, dateEntry := range dateDirs {
				if !dateEntry.IsDir() {
					continue
				}
				date := dateEntry.Name()

				// Validate date format (YYYY-MM-DD)
				if _, err := time.Parse("2006-01-02", date); err != nil {
					continue
				}

				// Filter by date
				if dateFilter != "" && date != dateFilter {
					continue
				}

				datePath := filepath.Join(providerPath, date)
				hourFiles, err := os.ReadDir(datePath)
				if err != nil {
					logrus.Warnf("Failed to read hour directory %s: %v", datePath, err)
					continue
				}

				for _, hourEntry := range hourFiles {
					if hourEntry.IsDir() {
						continue
					}

					hourFile := hourEntry.Name()
					// Validate filename format (HH.jsonl)
					if !strings.HasSuffix(hourFile, ".jsonl") {
						continue
					}
					hour := strings.TrimSuffix(hourFile, ".jsonl")
					if _, err := strconv.Atoi(hour); err != nil {
						continue
					}

					info, err := hourEntry.Info()
					if err != nil {
						continue
					}

					// Estimate count based on file size (rough estimate: ~1KB per entry)
					estimatedCount := int(info.Size() / 1024)
					if estimatedCount < 1 {
						estimatedCount = 1
					}

					recordings = append(recordings, RecordingFileInfo{
						Scenario:  scenario,
						Provider:  provider,
						Date:      date,
						Hour:      hour,
						Path:      filepath.Join(scenario, provider, date, hourFile),
						Size:      info.Size(),
						Count:     estimatedCount,
						CreatedAt: info.ModTime(),
					})
				}
			}
		}
	}

	return recordings, nil
}

// countLinesInFile counts the number of JSON lines in a file
func (s *Server) countLinesInFile(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	decoder := json.NewDecoder(file)

	for {
		var entry map[string]interface{}
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			// Skip malformed lines but continue counting
		}
		count++
	}

	return count, nil
}

// cleanupEmptyDirs removes empty directories
func (s *Server) cleanupEmptyDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	if len(entries) == 0 {
		os.Remove(dir)
	}
}

// RegisterRecordingRoutes registers recording-related routes
func (s *Server) RegisterRecordingRoutes(router *gin.RouterGroup) {
	recordings := router.Group("/recordings")
	{
		recordings.GET("", s.ListRecordings)                                              // List all recordings
		recordings.GET("/:scenario/:provider/:date/:hour", s.GetRecordingDetails)         // Get file details
		recordings.GET("/:scenario/:provider/:date/:hour/entries", s.GetRecordingEntries) // Get file entries
		recordings.DELETE("/:scenario/:provider/:date/:hour", s.DeleteRecording)          // Delete file
	}
}
