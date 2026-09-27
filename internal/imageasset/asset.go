// Package imageasset keeps the material image work is made from: prompt
// pieces (whole prompts, terms, phrases) and reference images. See
// .design/image-assets.md.
//
// It is deliberately self-contained so it can be split out of tingly-box
// later: it owns its own SQLite file and image directory under the directory
// it is given, and depends on nothing in tingly-box except the swagger route
// helper. Authentication is the host's business — the routes are registered
// on a group the host has already put behind its auth middleware.
package imageasset

import (
	"errors"
	"strings"
)

// Kind says how a piece is used: a whole prompt replaces the prompt; a term
// or a phrase is added to one.
type Kind string

const (
	KindPrompt Kind = "prompt"
	KindTerm   Kind = "term"
	KindPhrase Kind = "phrase"
)

func (k Kind) valid() bool {
	return k == KindPrompt || k == KindTerm || k == KindPhrase
}

// PromptPiece is one kept piece of prompt material.
type PromptPiece struct {
	ID    string   `json:"id"`
	Kind  Kind     `json:"kind" enum:"prompt,term,phrase"`
	Title string   `json:"title"`
	Text  string   `json:"text"`
	Tags  []string `json:"tags"`
	// SourceID is the whole prompt this piece was split from, if any. Not a
	// foreign key: deleting the prompt keeps the pieces.
	SourceID string `json:"source_id,omitempty"`
	// Unix milliseconds.
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
}

// PromptPieceInput creates a piece, or updates the one named by ID.
type PromptPieceInput struct {
	ID       string   `json:"id,omitempty"`
	Kind     Kind     `json:"kind" binding:"required" enum:"prompt,term,phrase"`
	Title    string   `json:"title,omitempty"`
	Text     string   `json:"text" binding:"required"`
	Tags     []string `json:"tags,omitempty"`
	SourceID string   `json:"source_id,omitempty"`
}

// ReferenceImage is one kept reference image. Its bytes are served separately
// (Store.ReferenceFile), so listing stays small.
type ReferenceImage struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	MIME   string `json:"mime"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Bytes  int64  `json:"bytes"`
	// Unix milliseconds.
	CreatedAt int64 `json:"created_at"`
}

// ReferenceInput is an image to keep: a name and its encoded bytes.
type ReferenceInput struct {
	Name string
	Data []byte
}

// Limits. Generous for a person's own material, small enough that one bad
// request cannot fill the disk or the database.
const (
	maxTextBytes     = 64 << 10
	maxTitleRunes    = 200
	maxNameRunes     = 200
	maxTags          = 32
	maxTagRunes      = 64
	maxBatch         = 200
	maxImageBytes    = 25 << 20
	maxImagesPerCall = 10
)

var (
	// ErrNotFound: no piece or reference with that id.
	ErrNotFound = errors.New("not found")
	// ErrInvalid wraps every validation failure, so the HTTP layer can tell a
	// bad request from a storage failure.
	ErrInvalid = errors.New("invalid")
)

func invalid(message string) error {
	return &validationError{message: message}
}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }
func (e *validationError) Unwrap() error { return ErrInvalid }

// normalizeTags lower-cases, trims and de-duplicates, keeping first-seen order.
func normalizeTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		value := strings.ToLower(strings.TrimSpace(tag))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// normalizePiece validates an input and returns it cleaned up: text and
// title trimmed, tags normalized, and the title dropped for terms and
// phrases, which are their own name.
func normalizePiece(in PromptPieceInput) (PromptPieceInput, error) {
	if !in.Kind.valid() {
		return in, invalid("kind must be prompt, term or phrase")
	}
	in.Text = strings.TrimSpace(in.Text)
	if in.Text == "" {
		return in, invalid("text is required")
	}
	if len(in.Text) > maxTextBytes {
		return in, invalid("text is too long")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Kind != KindPrompt {
		in.Title = ""
	}
	if len([]rune(in.Title)) > maxTitleRunes {
		return in, invalid("title is too long")
	}
	in.Tags = normalizeTags(in.Tags)
	if len(in.Tags) > maxTags {
		return in, invalid("too many tags")
	}
	for _, tag := range in.Tags {
		if len([]rune(tag)) > maxTagRunes {
			return in, invalid("tag is too long")
		}
	}
	return in, nil
}

func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", invalid("name is required")
	}
	if len([]rune(name)) > maxNameRunes {
		return "", invalid("name is too long")
	}
	return name, nil
}
