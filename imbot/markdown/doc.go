// Package markdown provides Telegram-compatible markdown rendering using MessageEntity.
//
// This package converts standard markdown to Telegram's entity-based format,
// eliminating the need for MarkdownV2 escaping and providing precise formatting control.
//
// Key features:
//   - No escape handling required
//   - UTF-16 offset calculation for correct entity positioning
//   - Support for bold, italic, code, code blocks, links, strikethrough, spoiler
//   - Long message splitting with entity preservation
//
// Example usage:
//
//	result, err := markdown.Convert("**Bold** and *italic* text")
//	if err != nil {
//	    // handle error
//	}
//	// Send to Telegram:
//	// text: "Bold and italic text"
//	// entities: [{type: "bold", offset: 0, length: 4}, {type: "italic", offset: 9, length: 6}]
//
// Reference: https://core.telegram.org/bots/api#messageentity
package markdown
