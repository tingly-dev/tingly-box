package markdown

import (
	"strings"
)

// ConvertResult holds the result of markdown conversion
type ConvertResult struct {
	Text     string          // Plain text without formatting
	Entities []MessageEntity // Formatting entities
}

// Convert converts markdown text to plain text with Telegram entities.
//
// This is the main entry point for markdown conversion. It parses the markdown
// and returns a result suitable for sending to Telegram Bot API without parse_mode.
//
// Example:
//
//	result, err := Convert("**Bold** and *italic*")
//	// result.Text: "Bold and italic"
//	// result.Entities: [{type: "bold", ...}, {type: "italic", ...}]
func Convert(markdown string) (*ConvertResult, error) {
	// Handle empty input
	if strings.TrimSpace(markdown) == "" {
		return &ConvertResult{
			Text:     "",
			Entities: []MessageEntity{},
		}, nil
	}

	// Parse markdown
	text, entities, err := parse(markdown)
	if err != nil {
		return nil, err
	}

	return &ConvertResult{
		Text:     text,
		Entities: entities,
	}, nil
}

// SplitEntities splits long text with entities into chunks.
//
// Telegram has a message length limit of 4096 UTF-16 code units.
// This function splits text at newline boundaries when possible,
// and adjusts entity offsets for each chunk.
//
// Entities that span a split boundary are clipped to fit within chunks.
//
// Algorithm:
//  1. Build UTF-16 offset table: Maps byte positions to UTF-16 code unit offsets
//  2. Find newline positions: Identifies potential split points for better readability
//  3. Determine chunk boundaries: Splits text at newlines when possible, hard-splits at UTF-16 boundaries otherwise
//  4. Adjust entities: Recalculates entity offsets and clips entities that cross chunk boundaries
func SplitEntities(text string, entities []MessageEntity, maxUTF16Len int) []ConvertResult {
	totalLen := UTF16Len(text)

	// If text fits in one message, return as-is
	if totalLen <= maxUTF16Len {
		return []ConvertResult{{
			Text:     text,
			Entities: entities,
		}}
	}

	// Pass 1: Build UTF-16 offset table for byte→UTF-16 position mapping
	offsets := UTF16OffsetTable(text)

	// Pass 2: Find newline positions (prefer splitting at newlines for readability)
	newlinePositions := findNewlinePositions(text)

	// Pass 3: Determine chunk boundaries based on UTF-16 limits and newlines
	chunks := determineChunks(text, offsets, newlinePositions, maxUTF16Len)

	// Pass 4: Build results with adjusted entities for each chunk
	results := make([]ConvertResult, len(chunks))
	for i, chunk := range chunks {
		chunkText := text[chunk.start:chunk.end]
		chunkEntities := adjustEntities(entities, offsets, chunk.start, chunk.end)

		results[i] = ConvertResult{
			Text:     chunkText,
			Entities: chunkEntities,
		}
	}

	return results
}

// chunk represents a text segment
type chunk struct {
	start int // byte offset start
	end   int // byte offset end
}

// findNewlinePositions finds all newline positions in text
func findNewlinePositions(text string) []int {
	positions := []int{}
	for i, r := range text {
		if r == '\n' {
			positions = append(positions, i+1) // Position after newline
		}
	}
	return positions
}

// determineChunks splits text into chunks at newline boundaries
func determineChunks(text string, offsets []int, newlines []int, maxLen int) []chunk {
	chunks := []chunk{}
	start := 0

	for start < len(text) {
		utf16Start := offsets[start]
		budget := utf16Start + maxLen

		// Check if remaining text fits
		if offsets[len(text)] <= budget {
			chunks = append(chunks, chunk{start: start, end: len(text)})
			break
		}

		// Find best split point (prefer newline)
		bestSplit := start
		for _, pos := range newlines {
			if pos <= start {
				continue
			}
			if pos >= len(offsets) {
				break
			}
			if offsets[pos] <= budget {
				bestSplit = pos
			} else {
				break
			}
		}

		// If no newline works, hard split at UTF-16 boundary
		if bestSplit == start {
			for i := start + 1; i <= len(text); i++ {
				if offsets[i] > budget {
					bestSplit = i - 1
					break
				}
			}
			if bestSplit == start {
				bestSplit = start + 1 // Force progress
			}
		}

		chunks = append(chunks, chunk{start: start, end: bestSplit})
		start = bestSplit
	}

	return chunks
}

// adjustEntities adjusts entity offsets for a chunk
func adjustEntities(entities []MessageEntity, offsets []int, chunkStart, chunkEnd int) []MessageEntity {
	chunkUTF16Start := offsets[chunkStart]
	chunkUTF16End := offsets[chunkEnd]

	adjusted := []MessageEntity{}

	for _, ent := range entities {
		entStart := ent.Offset
		entEnd := ent.Offset + ent.Length

		// Check overlap with chunk
		if entEnd <= chunkUTF16Start || entStart >= chunkUTF16End {
			continue // No overlap
		}

		// Clip to chunk boundaries
		clippedStart := max(entStart, chunkUTF16Start)
		clippedEnd := min(entEnd, chunkUTF16End)
		clippedLength := clippedEnd - clippedStart

		if clippedLength <= 0 {
			continue
		}

		// Adjust offset relative to chunk
		adjustedEnt := ent
		adjustedEnt.Offset = clippedStart - chunkUTF16Start
		adjustedEnt.Length = clippedLength

		adjusted = append(adjusted, adjustedEnt)
	}

	return adjusted
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
