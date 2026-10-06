package tracestore

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Trace is a stored trace with every body restored to bytes.
type Trace struct {
	*TraceRecord
	Inbound   *Message
	Exchanges []*Exchange
}

// Exchange is a stored exchange with restored bodies.
type Exchange struct {
	*ExchangeRecord
	Request  *Message
	Response *Message
}

// Message is a stored message with its restored body.
type Message struct {
	*MessageRecord
	Body []byte
}

// ReadPartition reads a partition data file and restores every trace in it,
// in write order.
func ReadPartition(path string) ([]*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	gz.Multistream(true)

	blocks := map[string]json.RawMessage{}
	var out []*Trace
	dec := json.NewDecoder(gz)
	for {
		var l line
		if err := dec.Decode(&l); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			// A torn final member (crash mid-write) ends the readable part.
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return out, nil
			}
			return out, err
		}
		switch l.Kind {
		case "blob":
			blocks[l.Hash] = l.Data
		case "trace":
			t, err := restore(l.TraceRecord, blocks)
			if err != nil {
				return out, fmt.Errorf("tracestore: trace %s: %w", l.TraceRecord.RequestID, err)
			}
			out = append(out, t)
		}
	}
}

// ReadIndex reads a partition's index file.
func ReadIndex(path string) ([]IndexEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []IndexEntry
	dec := json.NewDecoder(bufio.NewReader(f))
	for {
		var e IndexEntry
		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, e)
	}
}

func restore(rec *TraceRecord, blocks map[string]json.RawMessage) (*Trace, error) {
	t := &Trace{TraceRecord: rec}
	var err error
	if t.Inbound, err = restoreMessage(rec.Inbound, blocks); err != nil {
		return nil, err
	}
	for _, ex := range rec.Exchanges {
		e := &Exchange{ExchangeRecord: ex}
		if e.Request, err = restoreMessage(ex.Request, blocks); err != nil {
			return nil, err
		}
		if e.Response, err = restoreMessage(ex.Response, blocks); err != nil {
			return nil, err
		}
		t.Exchanges = append(t.Exchanges, e)
	}
	return t, nil
}

func restoreMessage(m *MessageRecord, blocks map[string]json.RawMessage) (*Message, error) {
	if m == nil {
		return nil, nil
	}
	body, err := decodeBody(m.Body, blocks)
	if err != nil {
		return nil, err
	}
	return &Message{MessageRecord: m, Body: body}, nil
}
