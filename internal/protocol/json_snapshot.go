package protocol

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// SnapshotJSON returns an owned JSON snapshot of one protocol value. SDK
// values may expose RawJSON to preserve fields unknown to their typed DTOs;
// typed nils are rejected before calling that method.
func SnapshotJSON(value any) ([]byte, error) {
	if isNilJSONValue(value) {
		return nil, fmt.Errorf("protocol JSON value %T is nil", value)
	}

	var raw []byte
	switch typed := value.(type) {
	case json.RawMessage:
		raw = typed
	case []byte:
		raw = typed
	case interface{ RawJSON() string }:
		raw = []byte(typed.RawJSON())
	}
	if len(raw) == 0 {
		var err error
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("protocol JSON value %T is not valid JSON", value)
	}
	return append([]byte(nil), raw...), nil
}

func isNilJSONValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
