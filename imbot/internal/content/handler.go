package content

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Handler handles specific content types from raw platform data
type Handler[RawT any] interface {
	// CanHandle determines if this handler can process the raw data
	CanHandle(raw RawT) bool

	// Handle processes the raw data and returns unified content
	Handle(ctx context.Context, raw RawT) (core.Content, error)

	// ContentType returns the content type this handler produces
	ContentType() string
}

// Registry manages content handlers
type Registry[RawT any] struct {
	handlers       []Handler[RawT]
	defaultHandler Handler[RawT]
}

// NewRegistry creates a new content handler registry
func NewRegistry[RawT any]() *Registry[RawT] {
	return &Registry[RawT]{
		handlers: make([]Handler[RawT], 0),
	}
}

// Register adds a content handler to the registry
func (r *Registry[RawT]) Register(handler Handler[RawT]) {
	r.handlers = append(r.handlers, handler)
}

// SetDefault sets the default handler for unknown content
func (r *Registry[RawT]) SetDefault(handler Handler[RawT]) {
	r.defaultHandler = handler
}

// Handle processes raw data using registered handlers
func (r *Registry[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	// Try each handler
	for _, handler := range r.handlers {
		if handler.CanHandle(raw) {
			content, err := handler.Handle(ctx, raw)
			if err != nil {
				return nil, fmt.Errorf("handler %s failed: %w", handler.ContentType(), err)
			}
			return content, nil
		}
	}

	// Fall back to default handler
	if r.defaultHandler != nil {
		return r.defaultHandler.Handle(ctx, raw)
	}

	// No handler found
	return nil, fmt.Errorf("no handler found for content type")
}

// TextHandler handles text content
type TextHandler[RawT any] struct {
	extractor func(RawT) (string, []core.Entity, bool)
}

func (h *TextHandler[RawT]) CanHandle(raw RawT) bool {
	_, _, ok := h.extractor(raw)
	return ok
}

func (h *TextHandler[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	text, entities, _ := h.extractor(raw)
	return core.NewTextContent(text, entities...), nil
}

func (h *TextHandler[RawT]) ContentType() string {
	return "text"
}

// NewTextHandler creates a text content handler
func NewTextHandler[RawT any](extractor func(RawT) (string, []core.Entity, bool)) Handler[RawT] {
	return &TextHandler[RawT]{extractor: extractor}
}

// MediaHandler handles media content (photos, documents, stickers, etc.)
type MediaHandler[RawT any] struct {
	mediaType string // "image", "video", "audio", "document", "sticker"
	extractor func(RawT) ([]core.MediaAttachment, string, bool)
}

func (h *MediaHandler[RawT]) CanHandle(raw RawT) bool {
	_, _, ok := h.extractor(raw)
	return ok // Only handle if extractor returns true
}

func (h *MediaHandler[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	media, caption, _ := h.extractor(raw)
	if len(media) == 0 {
		return nil, fmt.Errorf("no media found")
	}
	return core.NewMediaContent(media, caption), nil
}

func (h *MediaHandler[RawT]) ContentType() string {
	return h.mediaType
}

// NewMediaHandler creates a media content handler
func NewMediaHandler[RawT any](mediaType string, extractor func(RawT) ([]core.MediaAttachment, string, bool)) Handler[RawT] {
	return &MediaHandler[RawT]{mediaType: mediaType, extractor: extractor}
}

// PollHandler handles poll content
type PollHandler[RawT any] struct {
	extractor func(RawT) (core.Poll, bool)
}

func (h *PollHandler[RawT]) CanHandle(raw RawT) bool {
	_, ok := h.extractor(raw)
	return ok
}

func (h *PollHandler[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	poll, _ := h.extractor(raw)
	return core.NewPollContent(poll), nil
}

func (h *PollHandler[RawT]) ContentType() string {
	return "poll"
}

// NewPollHandler creates a poll content handler
func NewPollHandler[RawT any](extractor func(RawT) (core.Poll, bool)) Handler[RawT] {
	return &PollHandler[RawT]{extractor: extractor}
}

// SystemHandler handles system/unknown content
type SystemHandler[RawT any] struct {
	eventType string
	extractor func(RawT) (string, map[string]interface{}, bool)
}

func (h *SystemHandler[RawT]) CanHandle(raw RawT) bool {
	_, _, ok := h.extractor(raw)
	return ok
}

func (h *SystemHandler[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	eventType, data, _ := h.extractor(raw)
	return core.NewSystemContent(eventType, data), nil
}

func (h *SystemHandler[RawT]) ContentType() string {
	return "system"
}

// NewSystemHandler creates a system content handler
func NewSystemHandler[RawT any](eventType string, extractor func(RawT) (string, map[string]interface{}, bool)) Handler[RawT] {
	return &SystemHandler[RawT]{eventType: eventType, extractor: extractor}
}

// CompoundHandler combines multiple handlers (e.g., for messages with multiple media items)
type CompoundHandler[RawT any] struct {
	handlers []Handler[RawT]
}

func (h *CompoundHandler[RawT]) CanHandle(raw RawT) bool {
	for _, handler := range h.handlers {
		if handler.CanHandle(raw) {
			return true
		}
	}
	return false
}

func (h *CompoundHandler[RawT]) Handle(ctx context.Context, raw RawT) (core.Content, error) {
	for _, handler := range h.handlers {
		if handler.CanHandle(raw) {
			return handler.Handle(ctx, raw)
		}
	}
	return nil, fmt.Errorf("no sub-handler could process content")
}

func (h *CompoundHandler[RawT]) ContentType() string {
	return "compound"
}

// NewCompoundHandler creates a compound handler that tries multiple handlers
func NewCompoundHandler[RawT any](handlers ...Handler[RawT]) Handler[RawT] {
	return &CompoundHandler[RawT]{handlers: handlers}
}

// Helper functions for common content extraction

// ExtractText extracts text from a string field
func ExtractText[T any](field func(T) string) func(T) (string, []core.Entity, bool) {
	return func(raw T) (string, []core.Entity, bool) {
		text := field(raw)
		if text == "" {
			return "", nil, false
		}
		return text, nil, true
	}
}

// ExtractTextFromMap extracts text from a map with a key
func ExtractTextFromMap[T any](m map[string]interface{}, key string) func(T) (string, []core.Entity, bool) {
	return func(raw T) (string, []core.Entity, bool) {
		text, ok := m[key].(string)
		if !ok || text == "" {
			return "", nil, false
		}
		return text, nil, true
	}
}

// ExtractCaption extracts caption from fields with fallback
func ExtractCaption[T any](fields ...func(T) string) func(T) (string, bool) {
	return func(raw T) (string, bool) {
		for _, field := range fields {
			if caption := field(raw); caption != "" {
				return caption, true
			}
		}
		return "", false
	}
}

// CleanHTML cleans HTML tags (basic implementation)
func CleanHTML(html string) string {
	// Simple HTML tag removal
	result := html
	result = strings.ReplaceAll(result, "<b>", "")
	result = strings.ReplaceAll(result, "</b>", "")
	result = strings.ReplaceAll(result, "<i>", "")
	result = strings.ReplaceAll(result, "</i>", "")
	result = strings.ReplaceAll(result, "<u>", "")
	result = strings.ReplaceAll(result, "</u>", "")
	result = strings.ReplaceAll(result, "<s>", "")
	result = strings.ReplaceAll(result, "</s>", "")
	return result
}
