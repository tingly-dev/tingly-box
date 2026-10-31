// Package localsessions reads bounded previews of native Claude Code JSONL
// transcripts. It never runs agents or follows paths referenced by transcripts.
package localsessions

import "time"

const (
	PageSize            = 25
	MaxDirectoryEntries = 10000
	MaxReadBytes        = 2 << 20
	MaxRecordBytes      = 1 << 20
	MaxEvents           = 50
	MaxPageJSON         = 192 << 10
	MaxBlockText        = 4096
	MaxBlocks           = 8
)

type LocalSessionEntry struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Title    string    `json:"title,omitempty"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

type LocalSessionListing struct {
	Entries    []Entry `json:"entries"`
	NextCursor string  `json:"next_cursor,omitempty"`
	Total      int     `json:"total"`
}

type LocalSessionBlock struct {
	Kind      string `json:"kind"`
	Label     string `json:"label,omitempty"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type LocalSessionEvent struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	Type      string  `json:"type"`
	Timestamp string  `json:"timestamp,omitempty"`
	ParentID  string  `json:"parent_id,omitempty"`
	Sidechain bool    `json:"sidechain,omitempty"`
	Blocks    []Block `json:"blocks"`
}

type LocalSessionPage struct {
	Events         []Event `json:"events"`
	NextCursor     string  `json:"next_cursor,omitempty"`
	Direction      string  `json:"direction"`
	FileSize       int64   `json:"file_size"`
	ScannedBytes   int     `json:"scanned_bytes"`
	SkippedRecords int     `json:"skipped_records"`
}

type Entry = LocalSessionEntry

type Listing = LocalSessionListing

type Block = LocalSessionBlock

type Event = LocalSessionEvent

type Page = LocalSessionPage
