package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/sirupsen/logrus"
)

// StreamToStdin streams messages to the stdin of a running process.
func StreamToStdin(ctx context.Context, stdin io.WriteCloser, messages <-chan any) error {
	logrus.Debugln("[StreamToStdin] Starting to stream messages to stdin")

	// Use buffered writer for efficient I/O and ensure data is flushed
	writer := bufio.NewWriter(stdin)

	encoder := json.NewEncoder(writer)

	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			logrus.Debugln("[StreamToStdin] Context cancelled, stopping")
			return ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				// Channel closed, flush any remaining data and return
				// IMPORTANT: Do NOT close stdin here - it will be managed by the Query
				writer.Flush()
				logrus.Debugf("[StreamToStdin] Message channel closed after sending %d messages", messageCount)
				return nil
			}

			messageCount++
			logrus.Debugf("[StreamToStdin] Sending message #%d", messageCount)

			if err := encoder.Encode(msg); err != nil {
				return fmt.Errorf("encode message: %w", err)
			}
			// Flush immediately after each message to ensure prompt delivery
			if err := writer.Flush(); err != nil {
				return fmt.Errorf("flush message: %w", err)
			}
		}
	}
}

// StreamReader reads line-delimited JSON from a reader.
type StreamReader struct {
	scanner *bufio.Scanner
}

// NewStreamReader creates a new stream reader.
func NewStreamReader(r io.Reader) *StreamReader {
	return &StreamReader{
		scanner: bufio.NewScanner(r),
	}
}

// Next reads the next JSON object from the stream.
func (r *StreamReader) Next() (map[string]interface{}, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	line := r.scanner.Bytes()
	var data map[string]interface{}
	if err := json.Unmarshal(line, &data); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return data, nil
}

// ReadAll reads all remaining objects from the stream.
func (r *StreamReader) ReadAll() ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	for {
		data, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		results = append(results, data)
	}

	return results, nil
}

// StreamWriter writes line-delimited JSON to a writer.
type StreamWriter struct {
	writer io.Writer
	closed bool
	mu     sync.Mutex
}

// NewStreamWriter creates a new stream writer.
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{
		writer: w,
	}
}

// Write writes a JSON object to the stream.
func (w *StreamWriter) Write(data map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("stream writer is closed")
	}

	// Encode with newline
	buf, err := json.Marshal(data)
	if err != nil {
		return err
	}

	buf = append(buf, '\n')
	_, err = w.writer.Write(buf)
	return err
}

// Close closes the stream writer.
func (w *StreamWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.closed = true
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
