package tracestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// inlineThreshold is the size below which a top-level value stays inline in
// the skeleton: a block reference would cost about as much as the value.
const inlineThreshold = 256

// blockSink receives the blocks a body needs. put returns the block's hash
// and stores the block if this partition has not seen it yet.
type blockSink interface {
	put(data []byte) string
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// encodeBody turns raw body bytes into their stored form.
func encodeBody(data []byte, blocks blockSink) *Body {
	if data == nil {
		return nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		if fields, ok := objectMembers(trimmed); ok {
			b := &Body{Fields: make([]Field, 0, len(fields))}
			for _, m := range fields {
				b.Fields = append(b.Fields, encodeField(m.key, m.value, blocks))
			}
			return b
		}
	}
	if json.Valid(trimmed) && len(trimmed) > 0 {
		if len(trimmed) >= inlineThreshold {
			return &Body{Ref: blocks.put(trimmed)}
		}
		return &Body{Raw: append(json.RawMessage(nil), trimmed...)}
	}
	if utf8.Valid(data) {
		s := string(data)
		return &Body{Text: &s}
	}
	return &Body{B64: base64.StdEncoding.EncodeToString(data)}
}

func encodeField(key string, value json.RawMessage, blocks blockSink) Field {
	f := Field{Key: key}
	if len(value) < inlineThreshold {
		f.V = value
		return f
	}
	if value[0] == '[' {
		var elems []json.RawMessage
		if err := json.Unmarshal(value, &elems); err == nil {
			f.Chain, f.N = putChain(elems, blocks)
			return f
		}
	}
	f.Ref = blocks.put(value)
	return f
}

// putChain stores elems as a prefix chain and returns its head. Node k is
// {prev: node k-1, elem: hash(elems[k])}, so two bodies sharing a prefix
// share every node of it: appending to a history writes only the new tail.
func putChain(elems []json.RawMessage, blocks blockSink) (string, int) {
	prev := ""
	for _, e := range elems {
		node, _ := json.Marshal(chainNode{Prev: prev, Elem: blocks.put(e)})
		prev = blocks.put(node)
	}
	return prev, len(elems)
}

type member struct {
	key   string
	value json.RawMessage
}

// objectMembers splits a JSON object into its members in source order,
// keeping each value's raw bytes. ok is false for anything that is not
// exactly one well-formed object.
func objectMembers(data []byte) ([]member, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var out []member
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := tok.(string)
		if !ok {
			return nil, false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		out = append(out, member{key: key, value: v})
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, false
	}
	if _, err := dec.Token(); err == nil {
		return nil, false // trailing data
	}
	return out, true
}

// decodeBody restores a stored body to bytes. JSON bodies come back
// semantically equal to the original, compacted; elements keep their bytes.
func decodeBody(b *Body, blocks map[string]json.RawMessage) ([]byte, error) {
	switch {
	case b == nil:
		return nil, nil
	case b.Fields != nil:
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, f := range b.Fields {
			if i > 0 {
				buf.WriteByte(',')
			}
			key, _ := marshalNoEscape(f.Key)
			buf.Write(key)
			buf.WriteByte(':')
			v, err := decodeField(f, blocks)
			if err != nil {
				return nil, err
			}
			buf.Write(v)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case b.Ref != "":
		return lookup(blocks, b.Ref)
	case b.Raw != nil:
		return b.Raw, nil
	case b.Text != nil:
		return []byte(*b.Text), nil
	case b.B64 != "":
		return base64.StdEncoding.DecodeString(b.B64)
	}
	return nil, nil
}

func decodeField(f Field, blocks map[string]json.RawMessage) ([]byte, error) {
	switch {
	case f.Chain != "":
		elems := make([][]byte, f.N)
		node := f.Chain
		for i := f.N - 1; i >= 0; i-- {
			raw, err := lookup(blocks, node)
			if err != nil {
				return nil, err
			}
			var n chainNode
			if err := json.Unmarshal(raw, &n); err != nil {
				return nil, fmt.Errorf("tracestore: chain node %s: %w", node, err)
			}
			if elems[i], err = lookup(blocks, n.Elem); err != nil {
				return nil, err
			}
			node = n.Prev
		}
		if node != "" {
			return nil, fmt.Errorf("tracestore: chain %s longer than %d", f.Chain, f.N)
		}
		return append(append([]byte{'['}, bytes.Join(elems, []byte{','})...), ']'), nil
	case f.Ref != "":
		return lookup(blocks, f.Ref)
	default:
		return f.V, nil
	}
}

func lookup(blocks map[string]json.RawMessage, h string) ([]byte, error) {
	raw, ok := blocks[h]
	if !ok {
		return nil, fmt.Errorf("tracestore: missing block %s", h)
	}
	return raw, nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
