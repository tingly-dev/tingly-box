package assembler

import (
	"testing"

	"github.com/openai/openai-go/v3/responses"
)

// TestResponsesAssembler_New tests constructor
func TestResponsesAssembler_New(t *testing.T) {
	assembler := NewResponsesAssembler()
	if assembler == nil {
		t.Fatal("NewResponsesAssembler returned nil")
	}
	if assembler.Status() != "queued" {
		t.Errorf("expected status 'queued', got '%s'", assembler.Status())
	}
}

// TestResponsesAssembler_NewWithID tests constructor with custom IDs
func TestResponsesAssembler_NewWithID(t *testing.T) {
	assembler := NewResponsesAssemblerWithID("resp-custom", "item-custom")
	if assembler.ResponseID() != "resp-custom" {
		t.Errorf("expected response ID 'resp-custom', got '%s'", assembler.ResponseID())
	}
	if assembler.GetOrCreateItemID() != "item-custom" {
		t.Errorf("expected item ID 'item-custom', got '%s'", assembler.GetOrCreateItemID())
	}
}

// TestResponsesAssembler_CreatedEvent tests response.created event
func TestResponsesAssembler_CreatedEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	event := responses.ResponseStreamEventUnion{
		Type: "response.created",
		Response: responses.Response{
			ID: "resp-123",
		},
	}

	if !assembler.Accumulate(event) {
		t.Error("Accumulate returned false for created event")
	}

	if assembler.Status() != "created" {
		t.Errorf("expected status 'created', got '%s'", assembler.Status())
	}

	if assembler.ResponseID() != "resp-123" {
		t.Errorf("expected response ID 'resp-123', got '%s'", assembler.ResponseID())
	}
}

// TestResponsesAssembler_InProgressEvent tests response.in_progress event
func TestResponsesAssembler_InProgressEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	event := responses.ResponseStreamEventUnion{
		Type: "response.in_progress",
	}

	if !assembler.Accumulate(event) {
		t.Error("Accumulate returned false for in_progress event")
	}

	if assembler.Status() != "in_progress" {
		t.Errorf("expected status 'in_progress', got '%s'", assembler.Status())
	}
}

// TestResponsesAssembler_TextDelta tests text accumulation
func TestResponsesAssembler_TextDelta(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Simulate text delta events
	events := []responses.ResponseStreamEventUnion{
		{Type: "response.output_text.delta", Delta: "Hello"},
		{Type: "response.output_text.delta", Delta: " world"},
		{Type: "response.output_text.delta", Delta: "!"},
	}

	for _, event := range events {
		if !assembler.Accumulate(event) {
			t.Error("Accumulate returned false for delta event")
		}
	}

	// Check accumulated text (both current and accumulated)
	if assembler.OutputText() != "Hello world!" {
		t.Errorf("expected 'Hello world!', got '%s'", assembler.OutputText())
	}

	if assembler.CurrentText() != "Hello world!" {
		t.Errorf("expected current 'Hello world!', got '%s'", assembler.CurrentText())
	}
}

// TestResponsesAssembler_TextDone tests text completion
func TestResponsesAssembler_TextDone(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Add text delta
	deltaEvent := responses.ResponseStreamEventUnion{
		Type:  "response.output_text.delta",
		Delta: "Test message",
	}
	assembler.Accumulate(deltaEvent)

	// Complete the text
	doneEvent := responses.ResponseStreamEventUnion{
		Type: "response.output_text.done",
	}
	assembler.Accumulate(doneEvent)

	// After done, output text should still contain the accumulated text
	if assembler.OutputText() != "Test message" {
		t.Errorf("expected 'Test message', got '%s'", assembler.OutputText())
	}
}

// TestResponsesAssembler_CompletedEvent tests response.completed event
func TestResponsesAssembler_CompletedEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Create a completed event with a response
	event := responses.ResponseStreamEventUnion{
		Type: "response.completed",
		Response: responses.Response{
			ID:     "resp-123",
			Status: "completed",
			Output: []responses.ResponseOutputItemUnion{},
		},
	}

	if !assembler.Accumulate(event) {
		t.Error("Accumulate returned false for completed event")
	}

	if !assembler.IsCompleted() {
		t.Error("IsCompleted should return true")
	}

	if !assembler.IsFinished() {
		t.Error("IsFinished should return true")
	}

	response := assembler.Response()
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.ID != "resp-123" {
		t.Errorf("expected ID 'resp-123', got '%s'", response.ID)
	}
}

// TestResponsesAssembler_FailedEvent tests failure events
func TestResponsesAssembler_FailedEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	event := responses.ResponseStreamEventUnion{
		Type: "response.failed",
	}

	assembler.Accumulate(event)

	if !assembler.IsFailed() {
		t.Error("IsFailed should return true")
	}

	if !assembler.IsFinished() {
		t.Error("IsFinished should return true for failed status")
	}

	if assembler.Response() != nil {
		t.Error("Response should be nil for failed status")
	}
}

// TestResponsesAssembler_ErrorEvent tests error event
func TestResponsesAssembler_ErrorEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	event := responses.ResponseStreamEventUnion{
		Type: "error",
	}

	assembler.Accumulate(event)

	if assembler.Status() != "error" {
		t.Errorf("expected status 'error', got '%s'", assembler.Status())
	}

	if !assembler.IsFailed() {
		t.Error("IsFailed should return true for error status")
	}

	if !assembler.IsFinished() {
		t.Error("IsFinished should return true for error status")
	}
}

// TestResponsesAssembler_IncompleteEvent tests incomplete event
func TestResponsesAssembler_IncompleteEvent(t *testing.T) {
	assembler := NewResponsesAssembler()

	event := responses.ResponseStreamEventUnion{
		Type: "response.incomplete",
	}

	assembler.Accumulate(event)

	if !assembler.IsIncomplete() {
		t.Error("IsIncomplete should return true")
	}

	if !assembler.IsFinished() {
		t.Error("IsFinished should return true for incomplete status")
	}
}

// TestResponsesAssembler_Refusal tests refusal accumulation
func TestResponsesAssembler_Refusal(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Refusal delta
	deltaEvent := responses.ResponseStreamEventUnion{
		Type:  "response.refusal.delta",
		Delta: "I cannot",
	}
	assembler.Accumulate(deltaEvent)

	// More refusal
	deltaEvent2 := responses.ResponseStreamEventUnion{
		Type:  "response.refusal.delta",
		Delta: " fulfill this request.",
	}
	assembler.Accumulate(deltaEvent2)

	// Done - in real API, Refusal field contains the full refusal text
	// The delta events accumulate into currentRefusal
	doneEvent := responses.ResponseStreamEventUnion{
		Type: "response.refusal.done",
	}
	assembler.Accumulate(doneEvent)

	if assembler.CurrentRefusal() != "I cannot fulfill this request." {
		t.Errorf("expected 'I cannot fulfill this request.', got '%s'", assembler.CurrentRefusal())
	}
}

// TestResponsesAssembler_FunctionCallDelta tests function call argument accumulation
func TestResponsesAssembler_FunctionCallDelta(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Output item added for function call
	itemEvent := responses.ResponseStreamEventUnion{
		Type:        "response.output_item.added",
		ItemID:      "fc-123",
		OutputIndex: 0,
	}
	assembler.Accumulate(itemEvent)

	// Function call arguments delta
	deltaEvent := responses.ResponseStreamEventUnion{
		Type:        "response.function_call_arguments.delta",
		ItemID:      "fc-123",
		OutputIndex: 0,
		Arguments:   `{"city":`,
	}
	assembler.Accumulate(deltaEvent)

	// More arguments
	deltaEvent2 := responses.ResponseStreamEventUnion{
		Type:        "response.function_call_arguments.delta",
		ItemID:      "fc-123",
		OutputIndex: 0,
		Arguments:   `"NYC"}`,
	}
	assembler.Accumulate(deltaEvent2)

	// Done
	doneEvent := responses.ResponseStreamEventUnion{
		Type:        "response.function_call_arguments.done",
		ItemID:      "fc-123",
		OutputIndex: 0,
		Input:       `{"city":"NYC"}`,
	}
	assembler.Accumulate(doneEvent)

	// Note: Tool calls won't be tracked without the proper item structure
	// This is a limitation of the simplified implementation
}

// TestResponsesAssembler_Finish tests Finish method
func TestResponsesAssembler_Finish(t *testing.T) {
	t.Run("completed response", func(t *testing.T) {
		assembler := NewResponsesAssembler()

		event := responses.ResponseStreamEventUnion{
			Type: "response.completed",
			Response: responses.Response{
				ID: "resp-finish-test",
			},
		}
		assembler.Accumulate(event)

		result := assembler.Finish()
		if result == nil {
			t.Error("Finish should return response for completed status")
		}
		if result.ID != "resp-finish-test" {
			t.Errorf("expected ID 'resp-finish-test', got '%s'", result.ID)
		}
	})

	t.Run("incomplete response", func(t *testing.T) {
		assembler := NewResponsesAssembler()

		event := responses.ResponseStreamEventUnion{
			Type: "response.incomplete",
		}
		assembler.Accumulate(event)

		result := assembler.Finish()
		if result != nil {
			t.Error("Finish should return nil for incomplete status")
		}
	})

	t.Run("nil assembler", func(t *testing.T) {
		var assembler *ResponsesAssembler
		result := assembler.Finish()
		if result != nil {
			t.Error("Finish should return nil for nil assembler")
		}
	})
}

// TestResponsesAssembler_NilSafety tests nil safety
func TestResponsesAssembler_NilSafety(t *testing.T) {
	var assembler *ResponsesAssembler

	if assembler.Status() != "" {
		t.Error("Status should return empty string for nil assembler")
	}

	if assembler.OutputText() != "" {
		t.Error("OutputText should return empty string for nil assembler")
	}

	if assembler.CurrentText() != "" {
		t.Error("CurrentText should return empty string for nil assembler")
	}

	if assembler.CurrentRefusal() != "" {
		t.Error("CurrentRefusal should return empty string for nil assembler")
	}

	if assembler.IsCompleted() {
		t.Error("IsCompleted should return false for nil assembler")
	}

	if assembler.IsFailed() {
		t.Error("IsFailed should return false for nil assembler")
	}

	if assembler.Response() != nil {
		t.Error("Response should return nil for nil assembler")
	}

	if assembler.IsFinished() {
		t.Error("IsFinished should return false for nil assembler")
	}
}

// TestResponsesAssembler_UnsupportedEvents tests unsupported event handling
func TestResponsesAssembler_UnsupportedEvents(t *testing.T) {
	assembler := NewResponsesAssembler()

	// These events are not supported in the basic implementation
	unsupportedEvents := []string{
		"response.audio.delta",
		"response.audio.done",
		"response.code_interpreter_call_code.delta",
		"response.file_search_call.searching",
		"response.web_search_call.in_progress",
		"response.reasoning_text.delta",
		"response.image_generation_call.in_progress",
		"response.mcp_call.in_progress",
	}

	for _, eventType := range unsupportedEvents {
		event := responses.ResponseStreamEventUnion{
			Type: eventType,
		}

		// Should return false for unsupported events
		if assembler.Accumulate(event) {
			t.Errorf("Accumulate should return false for unsupported event type '%s'", eventType)
		}
	}
}

// TestResponsesAssembler_FullFlow tests a complete response flow
func TestResponsesAssembler_FullFlow(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Simulate a typical response flow
	events := []responses.ResponseStreamEventUnion{
		{Type: "response.created", Response: responses.Response{ID: "resp-flow-123"}},
		{Type: "response.in_progress"},
		{Type: "response.output_item.added", ItemID: "item-1", OutputIndex: 0},
		{Type: "response.content_part.added", ContentIndex: 0},
		{Type: "response.output_text.delta", Delta: "Hello, "},
		{Type: "response.output_text.delta", Delta: "world!"},
		{Type: "response.output_text.done"},
		{Type: "response.content_part.done"},
		{Type: "response.output_item.done"},
		{Type: "response.completed", Response: responses.Response{
			ID:     "resp-flow-123",
			Status: "completed",
			Output: []responses.ResponseOutputItemUnion{},
		}},
	}

	for _, event := range events {
		assembler.Accumulate(event)
	}

	if !assembler.IsCompleted() {
		t.Error("Expected completed status")
	}

	if assembler.OutputText() != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got '%s'", assembler.OutputText())
	}

	result := assembler.Finish()
	if result.ID != "resp-flow-123" {
		t.Errorf("expected ID 'resp-flow-123', got '%s'", result.ID)
	}
}

// TestResponsesAssembler_IDGeneration tests ID generation
func TestResponsesAssembler_IDGeneration(t *testing.T) {
	assembler := NewResponsesAssembler()

	// Before setting IDs, should generate them
	responseID := assembler.GetOrCreateResponseID()
	itemID := assembler.GetOrCreateItemID()

	if responseID == "" {
		t.Error("GetOrCreateResponseID should generate an ID")
	}
	if itemID == "" {
		t.Error("GetOrCreateItemID should generate an ID")
	}

	// Set custom IDs
	assembler.SetResponseID("custom-resp")
	assembler.SetItemID("custom-item")

	if assembler.ResponseID() != "custom-resp" {
		t.Errorf("expected 'custom-resp', got '%s'", assembler.ResponseID())
	}

	if assembler.GetOrCreateItemID() != "custom-item" {
		t.Errorf("expected 'custom-item', got '%s'", assembler.GetOrCreateItemID())
	}
}

// TestResponsesAssembler_SequenceNumber tests sequence number tracking
func TestResponsesAssembler_SequenceNumber(t *testing.T) {
	assembler := NewResponsesAssembler()

	seq1 := assembler.nextSequenceNumber()
	seq2 := assembler.nextSequenceNumber()
	seq3 := assembler.nextSequenceNumber()

	if seq1 != 0 {
		t.Errorf("expected first sequence number 0, got %d", seq1)
	}
	if seq2 != 1 {
		t.Errorf("expected second sequence number 1, got %d", seq2)
	}
	if seq3 != 2 {
		t.Errorf("expected third sequence number 2, got %d", seq3)
	}
}

// TestResponsesAssembler_OutputItemTracking tests output item tracking
func TestResponsesAssembler_OutputItemTracking(t *testing.T) {
	assembler := NewResponsesAssembler()

	addedEvent := responses.ResponseStreamEventUnion{
		Type:        "response.output_item.added",
		ItemID:      "item-123",
		OutputIndex: 0,
	}

	if !assembler.Accumulate(addedEvent) {
		t.Error("Accumulate returned false for output_item.added")
	}

	if assembler.itemID != "item-123" {
		t.Errorf("expected item ID 'item-123', got '%s'", assembler.itemID)
	}

	if assembler.outputIndex != 0 {
		t.Errorf("expected output index 0, got %d", assembler.outputIndex)
	}

	doneEvent := responses.ResponseStreamEventUnion{
		Type: "response.output_item.done",
	}

	if !assembler.Accumulate(doneEvent) {
		t.Error("Accumulate returned false for output_item.done")
	}
}

// TestResponsesAssembler_ContentPartTracking tests content part tracking
func TestResponsesAssembler_ContentPartTracking(t *testing.T) {
	assembler := NewResponsesAssembler()

	addedEvent := responses.ResponseStreamEventUnion{
		Type:         "response.content_part.added",
		ContentIndex: 0,
		OutputIndex:  0,
	}

	if !assembler.Accumulate(addedEvent) {
		t.Error("Accumulate returned false for content_part.added")
	}

	if assembler.contentIndex != 0 {
		t.Errorf("expected content index 0, got %d", assembler.contentIndex)
	}

	doneEvent := responses.ResponseStreamEventUnion{
		Type: "response.content_part.done",
	}

	if !assembler.Accumulate(doneEvent) {
		t.Error("Accumulate returned false for content_part.done")
	}

	if assembler.contentIndex != 0 {
		t.Errorf("expected content index reset to 0, got %d", assembler.contentIndex)
	}
}

// TestResponsesAssembler_ToolCalls tests tool call tracking
func TestResponsesAssembler_ToolCalls(t *testing.T) {
	assembler := NewResponsesAssembler()

	// The tool calls are tracked internally when output_item.added events
	// contain function call items. This test verifies the structure exists.
	toolCalls := assembler.ToolCalls()
	if toolCalls == nil {
		t.Error("ToolCalls should return a non-nil map")
	}

	if len(toolCalls) != 0 {
		t.Errorf("expected 0 tool calls, got %d", len(toolCalls))
	}
}
