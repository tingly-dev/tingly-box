package claude

import "encoding/json"

// Helper functions for type-safe map access (shared across package)

// marshalToMap serializes v to a map via JSON round-trip
func marshalToMap(v any) map[string]interface{} {
	data, _ := json.Marshal(v)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

// getString extracts a string value from a map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// getStringPtr extracts a string pointer from a map
func getStringPtr(m map[string]interface{}, key string) *string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return &str
		}
	}
	return nil
}

// getMap extracts a nested map from a map
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// getInt extracts an int value from a map
func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return int(f)
		}
	}
	return 0
}

// getInt64 extracts an int64 value from a map
func getInt64(m map[string]interface{}, key string) int64 {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return int64(f)
		}
	}
	return 0
}

// getFloat extracts a float64 value from a map
func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		if f, ok := val.(float64); ok {
			return f
		}
	}
	return 0
}

// getBool extracts a bool value from a map
func getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// intFromMap returns the first non-zero int found under any of keys. It exists
// for forward-compatible fields whose spelling varies across CLI versions.
func intFromMap(m map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v := getInt(m, k); v != 0 {
			return v
		}
	}
	return 0
}

// stringFromMap returns the first non-empty string found under any of keys.
func stringFromMap(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v := getString(m, k); v != "" {
			return v
		}
	}
	return ""
}
