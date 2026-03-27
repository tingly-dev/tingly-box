package markdown

import (
	"testing"
)

func TestConvert_Bold(t *testing.T) {
	result, err := Convert("**bold text**")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "bold text" {
		t.Errorf("Text = %q, want %q", result.Text, "bold text")
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeBold {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeBold)
	}
	if ent.Offset != 0 {
		t.Errorf("Entity offset = %d, want 0", ent.Offset)
	}
	if ent.Length != 9 {
		t.Errorf("Entity length = %d, want 9", ent.Length)
	}
}

func TestConvert_Italic(t *testing.T) {
	result, err := Convert("*italic text*")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "italic text" {
		t.Errorf("Text = %q, want %q", result.Text, "italic text")
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeItalic {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeItalic)
	}
}

func TestConvert_Code(t *testing.T) {
	result, err := Convert("`inline code`")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "inline code" {
		t.Errorf("Text = %q, want %q", result.Text, "inline code")
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeCode {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeCode)
	}
	if ent.Offset != 0 {
		t.Errorf("Entity offset = %d, want 0", ent.Offset)
	}
	if ent.Length != 11 {
		t.Errorf("Entity length = %d, want 11", ent.Length)
	}
}

func TestConvert_CodeBlock(t *testing.T) {
	markdown := "```go\nfunc main() {\n}\n```"
	result, err := Convert(markdown)
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	expectedText := "func main() {\n}"
	if result.Text != expectedText {
		t.Errorf("Text = %q, want %q", result.Text, expectedText)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypePre {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypePre)
	}
	if ent.Language == nil || *ent.Language != "go" {
		lang := ""
		if ent.Language != nil {
			lang = *ent.Language
		}
		t.Errorf("Entity language = %q, want %q", lang, "go")
	}
}

func TestConvert_Link(t *testing.T) {
	result, err := Convert("[Click here](https://example.com)")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "Click here" {
		t.Errorf("Text = %q, want %q", result.Text, "Click here")
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeTextLink {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeTextLink)
	}
	if ent.URL == nil || *ent.URL != "https://example.com" {
		url := ""
		if ent.URL != nil {
			url = *ent.URL
		}
		t.Errorf("Entity URL = %q, want %q", url, "https://example.com")
	}
}

func TestConvert_Mixed(t *testing.T) {
	result, err := Convert("**Bold**, *italic*, and `code`")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	expectedText := "Bold, italic, and code"
	if result.Text != expectedText {
		t.Errorf("Text = %q, want %q", result.Text, expectedText)
	}

	if len(result.Entities) != 3 {
		t.Fatalf("len(Entities) = %d, want 3", len(result.Entities))
	}

	// Check bold
	if result.Entities[0].Type != EntityTypeBold {
		t.Errorf("Entity[0] type = %q, want %q", result.Entities[0].Type, EntityTypeBold)
	}

	// Check italic
	if result.Entities[1].Type != EntityTypeItalic {
		t.Errorf("Entity[1] type = %q, want %q", result.Entities[1].Type, EntityTypeItalic)
	}

	// Check code
	if result.Entities[2].Type != EntityTypeCode {
		t.Errorf("Entity[2] type = %q, want %q", result.Entities[2].Type, EntityTypeCode)
	}
}

func TestConvert_Strikethrough(t *testing.T) {
	result, err := Convert("~~strikethrough~~")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "strikethrough" {
		t.Errorf("Text = %q, want %q", result.Text, "strikethrough")
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeStrikethrough {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeStrikethrough)
	}
}

func TestConvert_WithEmoji(t *testing.T) {
	result, err := Convert("**Hello 👍**")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	expectedText := "Hello 👍"
	if result.Text != expectedText {
		t.Errorf("Text = %q, want %q", result.Text, expectedText)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("len(Entities) = %d, want 1", len(result.Entities))
	}

	ent := result.Entities[0]
	if ent.Type != EntityTypeBold {
		t.Errorf("Entity type = %q, want %q", ent.Type, EntityTypeBold)
	}
	// "Hello 👍" = 5 + 1 space + 2 (emoji) = 8 UTF-16 units
	expectedLen := 8
	if ent.Length != expectedLen {
		t.Errorf("Entity length = %d, want %d", ent.Length, expectedLen)
	}
}

func TestConvert_Empty(t *testing.T) {
	result, err := Convert("")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "" {
		t.Errorf("Text = %q, want empty", result.Text)
	}

	if len(result.Entities) != 0 {
		t.Errorf("len(Entities) = %d, want 0", len(result.Entities))
	}
}

func TestConvert_PlainText(t *testing.T) {
	result, err := Convert("Just plain text")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if result.Text != "Just plain text" {
		t.Errorf("Text = %q, want %q", result.Text, "Just plain text")
	}

	if len(result.Entities) != 0 {
		t.Errorf("len(Entities) = %d, want 0", len(result.Entities))
	}
}

func TestSplitEntities(t *testing.T) {
	// Create a long text with entities
	text := "This is bold text. More text here. Even more text."
	entities := []MessageEntity{
		NewBoldEntity(8, 4), // "bold"
	}

	// Split with small max length
	results := SplitEntities(text, entities, 20)

	// Should split into multiple chunks
	if len(results) < 2 {
		t.Errorf("Expected multiple chunks, got %d", len(results))
	}

	// Verify first chunk has the bold entity
	if len(results) > 0 && len(results[0].Entities) == 0 {
		t.Error("Expected first chunk to have entity")
	}
}

func TestSplitEntities_FitsInOne(t *testing.T) {
	text := "Short text"
	entities := []MessageEntity{
		NewBoldEntity(0, 5),
	}

	results := SplitEntities(text, entities, 100)

	if len(results) != 1 {
		t.Errorf("Expected 1 chunk, got %d", len(results))
	}

	if results[0].Text != text {
		t.Errorf("Text = %q, want %q", results[0].Text, text)
	}

	if len(results[0].Entities) != 1 {
		t.Errorf("Expected 1 entity, got %d", len(results[0].Entities))
	}
}
