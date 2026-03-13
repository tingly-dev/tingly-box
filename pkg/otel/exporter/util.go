package exporter

import (
	"go.opentelemetry.io/otel/attribute"
)

// extractAttr extracts an attribute value from the attribute set.
func extractAttr(attrs attribute.Set, key string) string {
	val, ok := attrs.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return val.AsString()
}

// attrsToMap converts an attribute set to a map.
func attrsToMap(attrs attribute.Set) map[string]string {
	result := make(map[string]string)
	iter := attrs.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		result[string(kv.Key)] = kv.Value.Emit()
	}
	return result
}
