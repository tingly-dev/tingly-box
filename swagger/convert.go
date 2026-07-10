package swagger

import (
	"reflect"
	"strconv"
	"strings"
)

// parseValidationRules extracts validation constraints from a binding tag and
// applies them to the schema. goType must already be pointer-dereferenced.
func parseValidationRules(schema *Schema, bindingTag string, goType reflect.Type) {
	if bindingTag == "" {
		return
	}

	isString := goType.Kind() == reflect.String
	isSequence := goType.Kind() == reflect.Slice || goType.Kind() == reflect.Array

	setMinimum := func(value string, exclusive bool) {
		switch {
		case isString:
			if v, err := strconv.Atoi(value); err == nil {
				schema.MinLength = &v
			}
		case isSequence:
			if v, err := strconv.Atoi(value); err == nil {
				schema.MinItems = &v
			}
		default:
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Minimum = &v
				schema.ExclusiveMinimum = exclusive
			}
		}
	}
	setMaximum := func(value string, exclusive bool) {
		switch {
		case isString:
			if v, err := strconv.Atoi(value); err == nil {
				schema.MaxLength = &v
			}
		case isSequence:
			if v, err := strconv.Atoi(value); err == nil {
				schema.MaxItems = &v
			}
		default:
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				schema.Maximum = &v
				schema.ExclusiveMaximum = exclusive
			}
		}
	}

	for _, rule := range strings.Split(bindingTag, ",") {
		rule = strings.TrimSpace(rule)

		switch {
		case strings.HasPrefix(rule, "min="):
			setMinimum(strings.TrimPrefix(rule, "min="), false)
		case strings.HasPrefix(rule, "max="):
			setMaximum(strings.TrimPrefix(rule, "max="), false)
		case strings.HasPrefix(rule, "len="):
			value := strings.TrimPrefix(rule, "len=")
			setMinimum(value, false)
			setMaximum(value, false)
		case strings.HasPrefix(rule, "gte="):
			setMinimum(strings.TrimPrefix(rule, "gte="), false)
		case strings.HasPrefix(rule, "lte="):
			setMaximum(strings.TrimPrefix(rule, "lte="), false)
		case strings.HasPrefix(rule, "gt="):
			setMinimum(strings.TrimPrefix(rule, "gt="), true)
		case strings.HasPrefix(rule, "lt="):
			setMaximum(strings.TrimPrefix(rule, "lt="), true)
		case rule == "email":
			schema.Format = "email"
		case rule == "url", rule == "uri":
			schema.Format = "uri"
		case rule == "uuid":
			schema.Format = "uuid"
		case rule == "date":
			schema.Format = "date"
		case rule == "datetime":
			schema.Format = "date-time"
		case strings.HasPrefix(rule, "oneof="):
			enumValues := strings.Split(strings.TrimPrefix(rule, "oneof="), " ")
			schema.Enum = make([]interface{}, len(enumValues))
			for i, val := range enumValues {
				schema.Enum[i] = parseTaggedValue(val, goType)
			}
		case rule == "alphanum":
			schema.Pattern = "^[a-zA-Z0-9]+$"
		case rule == "alpha":
			schema.Pattern = "^[a-zA-Z]+$"
		case rule == "numeric":
			schema.Pattern = "^[0-9]+$"
		case rule == "hexadecimal":
			schema.Pattern = "^[0-9a-fA-F]+$"
		case strings.HasPrefix(rule, "contains="):
			if isString {
				schema.Pattern = ".*" + strings.TrimPrefix(rule, "contains=") + ".*"
			}
		case strings.HasPrefix(rule, "startswith="):
			if isString {
				schema.Pattern = "^" + strings.TrimPrefix(rule, "startswith=") + ".*"
			}
		case strings.HasPrefix(rule, "endswith="):
			if isString {
				schema.Pattern = ".*" + strings.TrimPrefix(rule, "endswith=") + "$"
			}
		}
	}
}

// parseTaggedValue parses a tag-provided literal (example, default, enum
// entry) into a value matching the field type.
func parseTaggedValue(value string, goType reflect.Type) interface{} {
	goType = derefType(goType)

	switch goType.Kind() {
	case reflect.String:
		return value
	case reflect.Bool:
		if val, err := strconv.ParseBool(value); err == nil {
			return val
		}
		return false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if val, err := strconv.ParseInt(value, 10, 64); err == nil {
			return val
		}
		return int64(0)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if val, err := strconv.ParseUint(value, 10, 64); err == nil {
			return val
		}
		return uint64(0)
	case reflect.Float32, reflect.Float64:
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			return val
		}
		return 0.0
	case reflect.Slice, reflect.Array:
		if goType.Elem().Kind() == reflect.String {
			return strings.Split(value, ",")
		}
		return []interface{}{}
	default:
		return value
	}
}

// queryParamType maps a Go type to the (type, format) pair used for query
// parameters. Query parameters are always primitive-ish; struct and map types
// degrade to "object" as before.
func queryParamType(goType reflect.Type) (string, string) {
	goType = derefType(goType)

	switch goType.Kind() {
	case reflect.String:
		return "string", ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer", "int64"
	case reflect.Float32, reflect.Float64:
		return "number", "double"
	case reflect.Bool:
		return "boolean", ""
	case reflect.Slice, reflect.Array:
		return "array", ""
	case reflect.Struct:
		if goType == timeType {
			return "string", "date-time"
		}
		return "object", ""
	default:
		return "object", ""
	}
}
