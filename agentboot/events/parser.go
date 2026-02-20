package events

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Parser parses streaming agent output
type Parser struct {
	eventChan chan Event
	done      chan struct{}
	once      sync.Once
}

// NewParser creates a new event parser
func NewParser() *Parser {
	return &Parser{
		eventChan: make(chan Event, 100),
		done:      make(chan struct{}),
	}
}

// NewParserWithBufferSize creates a new event parser with custom buffer size
func NewParserWithBufferSize(bufferSize int) *Parser {
	return &Parser{
		eventChan: make(chan Event, bufferSize),
		done:      make(chan struct{}),
	}
}

// Parse reads JSON lines and emits events
func (p *Parser) Parse(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)

	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			// Not JSON, treat as text line
			p.emit(Event{
				Type: "text",
				Data: map[string]interface{}{
					"text": line,
				},
				Timestamp: time.Now(),
				Raw:       line,
			})
			continue
		}

		// Create event from parsed data
		p.emit(NewEventFromRaw(line, data))
	}

	p.close()
	return scanner.Err()
}

// Events returns the event channel
func (p *Parser) Events() <-chan Event {
	return p.eventChan
}

// Close closes the parser
func (p *Parser) Close() {
	p.close()
}

func (p *Parser) emit(event Event) {
	select {
	case p.eventChan <- event:
	case <-p.done:
	}
}

func (p *Parser) close() {
	p.once.Do(func() {
		close(p.eventChan)
		close(p.done)
	})
}

// CollectAll collects all events until the parser closes
func (p *Parser) CollectAll() []Event {
	var events []Event
	for event := range p.Events() {
		events = append(events, event)
	}
	return events
}

// StreamToHandler streams events to an event handler
func (p *Parser) StreamToHandler(handler EventHandler) error {
	for event := range p.Events() {
		if err := handler.HandleEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// StreamToListener streams events to a listener
func (p *Parser) StreamToListener(listener Listener) {
	for event := range p.Events() {
		listener.OnEvent(event)
	}
}
