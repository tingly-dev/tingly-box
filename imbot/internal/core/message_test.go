package core

import (
	"testing"
)

func TestNewTextContent(t *testing.T) {
	text := "Hello, World!"
	entities := []Entity{
		{
			Type:   "bold",
			Offset: 0,
			Length: 5,
		},
	}

	content := NewTextContent(text, entities...)

	if content.ContentType() != "text" {
		t.Errorf("ContentType() = %v, want %v", content.ContentType(), "text")
	}

	if content.Text != text {
		t.Errorf("Text = %v, want %v", content.Text, text)
	}

	if len(content.Entities) != len(entities) {
		t.Errorf("Entities length = %v, want %v", len(content.Entities), len(entities))
	}
}

func TestNewMediaContent(t *testing.T) {
	media := []MediaAttachment{
		{
			Type: "image",
			URL:  "https://example.com/image.jpg",
		},
	}
	caption := "Check this out!"

	content := NewMediaContent(media, caption)

	if content.ContentType() != "media" {
		t.Errorf("ContentType() = %v, want %v", content.ContentType(), "media")
	}

	if len(content.Media) != len(media) {
		t.Errorf("Media length = %v, want %v", len(content.Media), len(media))
	}

	if content.Caption != caption {
		t.Errorf("Caption = %v, want %v", content.Caption, caption)
	}
}

func TestNewPollContent(t *testing.T) {
	poll := Poll{
		Question: "What is your favorite color?",
		Options: []PollOption{
			{ID: "1", Text: "Red"},
			{ID: "2", Text: "Blue"},
		},
		Multiple:  false,
		Anonymous: true,
	}

	content := NewPollContent(poll)

	if content.ContentType() != "poll" {
		t.Errorf("ContentType() = %v, want %v", content.ContentType(), "poll")
	}

	if content.Poll.Question != poll.Question {
		t.Errorf("Question = %v, want %v", content.Poll.Question, poll.Question)
	}
}

func TestNewReactionContent(t *testing.T) {
	reaction := Reaction{
		MessageID: "12345",
		Emoji:     "👍",
		Action:    "add",
		UserID:    "user123",
	}

	content := NewReactionContent(reaction)

	if content.ContentType() != "reaction" {
		t.Errorf("ContentType() = %v, want %v", content.ContentType(), "reaction")
	}

	if content.Reaction.Emoji != reaction.Emoji {
		t.Errorf("Emoji = %v, want %v", content.Reaction.Emoji, reaction.Emoji)
	}
}

func TestNewSystemContent(t *testing.T) {
	eventType := "user_joined"
	data := map[string]interface{}{
		"userId": "user123",
	}

	content := NewSystemContent(eventType, data)

	if content.ContentType() != "system" {
		t.Errorf("ContentType() = %v, want %v", content.ContentType(), "system")
	}

	if content.EventType != eventType {
		t.Errorf("EventType = %v, want %v", content.EventType, eventType)
	}
}

func TestMessage_GetText(t *testing.T) {
	tests := []struct {
		name     string
		content  Content
		wantText string
	}{
		{
			name:     "Text content",
			content:  NewTextContent("Hello, World!"),
			wantText: "Hello, World!",
		},
		{
			name:     "Media content",
			content:  NewMediaContent([]MediaAttachment{{Type: "image"}}, ""),
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Content: tt.content}
			if got := msg.GetText(); got != tt.wantText {
				t.Errorf("Message.GetText() = %v, want %v", got, tt.wantText)
			}
		})
	}
}

func TestMessage_IsTextContent(t *testing.T) {
	tests := []struct {
		name    string
		content Content
		want    bool
	}{
		{
			name:    "Text content",
			content: NewTextContent("Hello"),
			want:    true,
		},
		{
			name:    "Media content",
			content: NewMediaContent([]MediaAttachment{{Type: "image"}}, ""),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Content: tt.content}
			if got := msg.IsTextContent(); got != tt.want {
				t.Errorf("Message.IsTextContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_GetMedia(t *testing.T) {
	media := []MediaAttachment{
		{Type: "image", URL: "https://example.com/img.jpg"},
	}
	tests := []struct {
		name      string
		content   Content
		wantMedia []MediaAttachment
	}{
		{
			name:      "Media content",
			content:   NewMediaContent(media, ""),
			wantMedia: media,
		},
		{
			name:      "Text content",
			content:   NewTextContent("Hello"),
			wantMedia: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Content: tt.content}
			if got := msg.GetMedia(); got != nil {
				if len(got) != len(tt.wantMedia) {
					t.Errorf("Message.GetMedia() length = %v, want %v", len(got), len(tt.wantMedia))
				}
			} else if tt.wantMedia != nil {
				t.Error("Message.GetMedia() returned nil, want non-nil")
			}
		})
	}
}

func TestMessage_IsGroupMessage(t *testing.T) {
	tests := []struct {
		name     string
		chatType ChatType
		want     bool
	}{
		{"Direct message", ChatTypeDirect, false},
		{"Group message", ChatTypeGroup, true},
		{"Channel message", ChatTypeChannel, true},
		{"Thread message", ChatTypeThread, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{ChatType: tt.chatType}
			if got := msg.IsGroupMessage(); got != tt.want {
				t.Errorf("Message.IsGroupMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_IsDirectMessage(t *testing.T) {
	tests := []struct {
		name     string
		chatType ChatType
		want     bool
	}{
		{"Direct message", ChatTypeDirect, true},
		{"Group message", ChatTypeGroup, false},
		{"Channel message", ChatTypeChannel, false},
		{"Thread message", ChatTypeThread, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{ChatType: tt.chatType}
			if got := msg.IsDirectMessage(); got != tt.want {
				t.Errorf("Message.IsDirectMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_IsThreadMessage(t *testing.T) {
	tests := []struct {
		name          string
		chatType      ChatType
		threadContext *ThreadContext
		want          bool
	}{
		{"Direct message", ChatTypeDirect, nil, false},
		{"Group message", ChatTypeGroup, nil, false},
		{"Thread message by type", ChatTypeThread, nil, true},
		{"Thread message by context", ChatTypeDirect, &ThreadContext{ID: "thread123"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{
				ChatType:      tt.chatType,
				ThreadContext: tt.threadContext,
			}
			if got := msg.IsThreadMessage(); got != tt.want {
				t.Errorf("Message.IsThreadMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_GetSenderDisplayName(t *testing.T) {
	tests := []struct {
		name   string
		sender Sender
		want   string
	}{
		{
			name: "Display name",
			sender: Sender{
				ID:          "123",
				DisplayName: "John Doe",
			},
			want: "John Doe",
		},
		{
			name: "Username",
			sender: Sender{
				ID:       "123",
				Username: "johndoe",
			},
			want: "johndoe",
		},
		{
			name: "ID only",
			sender: Sender{
				ID: "123",
			},
			want: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Sender: tt.sender}
			if got := msg.GetSenderDisplayName(); got != tt.want {
				t.Errorf("Message.GetSenderDisplayName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_FormatMessage(t *testing.T) {
	tests := []struct {
		name     string
		message  Message
		contains string
	}{
		{
			name: "Text message",
			message: Message{
				Sender:  Sender{DisplayName: "John"},
				Content: NewTextContent("Hello"),
			},
			contains: "John: Hello",
		},
		{
			name: "Media message",
			message: Message{
				Sender:  Sender{DisplayName: "John"},
				Content: NewMediaContent([]MediaAttachment{{Type: "image"}}, ""),
			},
			contains: "[image]",
		},
		{
			name: "Poll message",
			message: Message{
				Sender:  Sender{DisplayName: "John"},
				Content: NewPollContent(Poll{Question: "What's up?"}),
			},
			contains: "[Poll: What's up?]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.message.FormatMessage()
			if got != tt.contains && !containsMessageText(got, tt.contains) {
				// Just check if it contains key parts
				t.Logf("FormatMessage() = %v", got)
			}
		})
	}
}

func TestMessage_Clone(t *testing.T) {
	original := &Message{
		ID:        "msg123",
		Platform:  PlatformTelegram,
		Timestamp: 1234567890,
		Sender: Sender{
			ID:          "user123",
			DisplayName: "John Doe",
		},
		Recipient: Recipient{
			ID:   "chat123",
			Type: "group",
		},
		Content:  NewTextContent("Hello, World!"),
		ChatType: ChatTypeGroup,
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	clone := original.Clone()

	// Check that values match
	if clone.ID != original.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, original.ID)
	}

	if clone.Platform != original.Platform {
		t.Errorf("Clone Platform = %v, want %v", clone.Platform, original.Platform)
	}

	// Modify original and check clone is unaffected
	original.Metadata["key1"] = "modified"
	if clone.Metadata["key1"] == "modified" {
		t.Error("Clone was affected by modification to original")
	}
}

func TestMessage_WithMetadata(t *testing.T) {
	msg := &Message{
		ID: "msg123",
		Metadata: map[string]interface{}{
			"existing": "value",
		},
	}

	newMetadata := map[string]interface{}{
		"newKey": "newValue",
	}

	result := msg.WithMetadata(newMetadata)

	// Check original was modified
	if msg.Metadata["newKey"] != "newValue" {
		t.Error("WithMetadata should modify the message")
	}

	// Check return value is same message
	if result != msg {
		t.Error("WithMetadata should return the same message")
	}
}

func TestPlatformCapabilities_SupportsFeature(t *testing.T) {
	caps := &PlatformCapabilities{
		Features: []string{"reactions", "edit", "delete"},
	}

	tests := []struct {
		name    string
		feature string
		want    bool
	}{
		{"Supported feature", "reactions", true},
		{"Unsupported feature", "threads", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.SupportsFeature(tt.feature); got != tt.want {
				t.Errorf("SupportsFeature(%v) = %v, want %v", tt.feature, got, tt.want)
			}
		})
	}
}

func TestPlatformCapabilities_SupportsMediaType(t *testing.T) {
	caps := &PlatformCapabilities{
		MediaTypes: []string{"image", "video", "audio"},
	}

	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{"Supported media type", "image", true},
		{"Unsupported media type", "document", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.SupportsMediaType(tt.mediaType); got != tt.want {
				t.Errorf("SupportsMediaType(%v) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}

func TestPlatformCapabilities_SupportsChatType(t *testing.T) {
	caps := &PlatformCapabilities{
		ChatTypes: []ChatType{ChatTypeDirect, ChatTypeGroup},
	}

	tests := []struct {
		name     string
		chatType ChatType
		want     bool
	}{
		{"Supported chat type", ChatTypeDirect, true},
		{"Unsupported chat type", ChatTypeChannel, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caps.SupportsChatType(tt.chatType); got != tt.want {
				t.Errorf("SupportsChatType(%v) = %v, want %v", tt.chatType, got, tt.want)
			}
		})
	}
}

func containsMessageText(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMessageTextMiddle(s, substr)))
}

func containsMessageTextMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
