package markdown

import (
	"log"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	eastast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// walker walks the goldmark AST and produces plain text with entities
type walker struct {
	source   []byte          // Original markdown source
	buf      strings.Builder // Plain text output
	entities []MessageEntity // Collected entities
	utf16Pos int             // Current UTF-16 position
	stack    []pendingEntity // Stack for nested entities
}

// pendingEntity tracks an entity that hasn't been closed yet
type pendingEntity struct {
	entityType string
	startPos   int    // UTF-16 start position
	url        string // For text_link entities
	language   string // For pre entities
}

// parse converts markdown to plain text and entities
func parse(markdown string) (string, []MessageEntity, error) {
	// Create goldmark parser with extensions
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Strikethrough,
			extension.Table,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	// Parse markdown to AST
	source := []byte(markdown)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	// Walk AST and build result
	w := &walker{
		source:   source,
		entities: make([]MessageEntity, 0),
		stack:    make([]pendingEntity, 0),
	}

	if err := ast.Walk(doc, w.walkNode); err != nil {
		return "", nil, err
	}

	return w.buf.String(), w.entities, nil
}

// walkNode processes a single AST node
func (w *walker) walkNode(node ast.Node, entering bool) (ast.WalkStatus, error) {
	switch n := node.(type) {
	case *ast.Document:
		// Root node, continue
		return ast.WalkContinue, nil

	case *ast.Paragraph:
		if !entering {
			// Add blank line after paragraph (unless it's the last node)
			if node.NextSibling() != nil {
				w.writeText("\n\n")
			}
		}
		return ast.WalkContinue, nil

	case *ast.Heading:
		if entering {
			// Add blank line before heading
			if node.PreviousSibling() != nil {
				w.writeText("\n\n")
			}
			// Make heading bold
			w.pushEntity(EntityTypeBold, "", "")
		} else {
			w.popEntity()
		}
		return ast.WalkContinue, nil

	case *ast.Emphasis:
		if entering {
			// Level 1 = italic (*), Level 2 = bold (**)
			if n.Level == 1 {
				w.pushEntity(EntityTypeItalic, "", "")
			} else {
				w.pushEntity(EntityTypeBold, "", "")
			}
		} else {
			w.popEntity()
		}
		return ast.WalkContinue, nil

	case *ast.CodeSpan:
		if entering {
			start := w.utf16Pos
			// Extract code text
			code := string(n.Text(w.source))
			w.writeText(code)
			length := w.utf16Pos - start
			if length > 0 {
				w.entities = append(w.entities, NewCodeEntity(start, length))
			}
		}
		return ast.WalkSkipChildren, nil

	case *ast.FencedCodeBlock:
		if entering {
			start := w.utf16Pos

			// Add blank line before code block
			if node.PreviousSibling() != nil {
				w.writeText("\n\n")
			}

			// Extract code lines
			lines := n.Lines()
			var code strings.Builder
			for i := 0; i < lines.Len(); i++ {
				line := lines.At(i)
				if i > 0 {
					code.WriteString("\n")
				}
				// Trim trailing newline from each line
				lineText := line.Value(w.source)
				lineText = []byte(strings.TrimSuffix(string(lineText), "\n"))
				code.Write(lineText)
			}
			codeText := code.String()
			w.writeText(codeText)

			length := w.utf16Pos - start
			if length > 0 {
				// Extract language
				lang := string(n.Language(w.source))
				w.entities = append(w.entities, NewPreEntity(start, length, lang))
			}
		}
		return ast.WalkSkipChildren, nil

	case *ast.Link:
		if entering {
			dest := string(n.Destination)
			w.pushEntity(EntityTypeTextLink, dest, "")
		} else {
			w.popEntity()
		}
		return ast.WalkContinue, nil

	case *ast.Text:
		if entering {
			// Regular text node
			txt := string(n.Text(w.source))

			// Handle soft line breaks
			if n.SoftLineBreak() {
				txt += "\n"
			}

			// Handle hard line breaks
			if n.HardLineBreak() {
				txt += "\n"
			}

			w.writeText(txt)
		}
		return ast.WalkSkipChildren, nil

	case *ast.String:
		if entering {
			w.writeText(string(n.Value))
		}
		return ast.WalkSkipChildren, nil

	case *eastast.Strikethrough:
		if entering {
			w.pushEntity(EntityTypeStrikethrough, "", "")
		} else {
			w.popEntity()
		}
		return ast.WalkContinue, nil

	case *ast.Blockquote:
		if entering {
			if node.PreviousSibling() != nil {
				w.writeText("\n\n")
			}
			w.pushEntity(EntityTypeBlockquote, "", "")
		} else {
			w.popEntity()
		}
		return ast.WalkContinue, nil

	case *ast.List:
		if !entering && node.NextSibling() != nil {
			w.writeText("\n")
		}
		return ast.WalkContinue, nil

	case *ast.ListItem:
		if entering {
			// Add bullet or number
			parent := node.Parent()
			if list, ok := parent.(*ast.List); ok {
				if list.IsOrdered() {
					// Find item index
					idx := 1
					for prev := node.PreviousSibling(); prev != nil; prev = prev.PreviousSibling() {
						idx++
					}
					w.writeText(string(rune('0' + idx)))
					w.writeText(". ")
				} else {
					w.writeText("• ")
				}
			}
		} else {
			if node.NextSibling() != nil {
				w.writeText("\n")
			}
		}
		return ast.WalkContinue, nil

	case *ast.ThematicBreak:
		if entering {
			if node.PreviousSibling() != nil {
				w.writeText("\n\n")
			}
			w.writeText("────────")
		}
		return ast.WalkSkipChildren, nil
	}

	return ast.WalkContinue, nil
}

// writeText writes text and updates UTF-16 position
func (w *walker) writeText(text string) {
	w.buf.WriteString(text)
	w.utf16Pos += UTF16Len(text)
}

// pushEntity starts tracking a new entity
func (w *walker) pushEntity(entityType, url, language string) {
	w.stack = append(w.stack, pendingEntity{
		entityType: entityType,
		startPos:   w.utf16Pos,
		url:        url,
		language:   language,
	})
}

// popEntity closes the most recent entity
func (w *walker) popEntity() {
	if len(w.stack) == 0 {
		// Malformed markdown may cause unbalanced entities (e.g., **bold *italic**)
		log.Printf("[markdown] Warning: popEntity called with empty stack - markdown may be malformed")
		return
	}

	// Pop from stack
	pending := w.stack[len(w.stack)-1]
	w.stack = w.stack[:len(w.stack)-1]

	// Calculate length
	length := w.utf16Pos - pending.startPos
	if length <= 0 {
		return
	}

	// Create entity based on type
	var entity MessageEntity
	switch pending.entityType {
	case EntityTypeTextLink:
		entity = NewTextLinkEntity(pending.startPos, length, pending.url)
	case EntityTypePre:
		entity = NewPreEntity(pending.startPos, length, pending.language)
	case EntityTypeBold:
		entity = NewBoldEntity(pending.startPos, length)
	case EntityTypeItalic:
		entity = NewItalicEntity(pending.startPos, length)
	case EntityTypeStrikethrough:
		entity = NewStrikethroughEntity(pending.startPos, length)
	case EntityTypeBlockquote:
		entity = NewBlockquoteEntity(pending.startPos, length)
	default:
		return
	}

	w.entities = append(w.entities, entity)
}
