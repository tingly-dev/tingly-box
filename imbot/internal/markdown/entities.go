package markdown

// MessageEntity represents a Telegram message entity.
//
// Entities provide formatting information for parts of the message text.
// Offset and Length are measured in UTF-16 code units, not bytes or runes.
//
// See: https://core.telegram.org/bots/api#messageentity
type MessageEntity struct {
	Type     string  `json:"type"`               // Entity type (bold, italic, code, etc.)
	Offset   int     `json:"offset"`             // UTF-16 offset where the entity starts
	Length   int     `json:"length"`             // UTF-16 length of the entity
	URL      *string `json:"url,omitempty"`      // For text_link entities
	Language *string `json:"language,omitempty"` // For pre entities (code blocks)
}

// Supported entity types
const (
	EntityTypeBold          = "bold"
	EntityTypeItalic        = "italic"
	EntityTypeUnderline     = "underline"
	EntityTypeStrikethrough = "strikethrough"
	EntityTypeSpoiler       = "spoiler"
	EntityTypeCode          = "code"       // Inline code
	EntityTypePre           = "pre"        // Code block
	EntityTypeTextLink      = "text_link"  // Link with custom text
	EntityTypeBlockquote    = "blockquote" // Block quote
)

// NewBoldEntity creates a bold entity
func NewBoldEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeBold,
		Offset: offset,
		Length: length,
	}
}

// NewItalicEntity creates an italic entity
func NewItalicEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeItalic,
		Offset: offset,
		Length: length,
	}
}

// NewCodeEntity creates an inline code entity
func NewCodeEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeCode,
		Offset: offset,
		Length: length,
	}
}

// NewPreEntity creates a code block entity with optional language
func NewPreEntity(offset, length int, language string) MessageEntity {
	var lang *string
	if language != "" {
		lang = &language
	}
	return MessageEntity{
		Type:     EntityTypePre,
		Offset:   offset,
		Length:   length,
		Language: lang,
	}
}

// NewTextLinkEntity creates a link entity
func NewTextLinkEntity(offset, length int, url string) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeTextLink,
		Offset: offset,
		Length: length,
		URL:    &url,
	}
}

// NewStrikethroughEntity creates a strikethrough entity
func NewStrikethroughEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeStrikethrough,
		Offset: offset,
		Length: length,
	}
}

// NewSpoilerEntity creates a spoiler entity
func NewSpoilerEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeSpoiler,
		Offset: offset,
		Length: length,
	}
}

// NewBlockquoteEntity creates a blockquote entity
func NewBlockquoteEntity(offset, length int) MessageEntity {
	return MessageEntity{
		Type:   EntityTypeBlockquote,
		Offset: offset,
		Length: length,
	}
}
