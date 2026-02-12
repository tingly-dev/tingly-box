package swagger

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// getSwaggerTypeWithDetails converts Go type to Swagger schema with additional details
func (rm *RouteManager) getSwaggerTypeWithDetails(goType reflect.Type, bindingTag, formatTag, enumTag string) Schema {
	schema := rm.getSwaggerType(goType)

	// Override format if explicitly specified
	if formatTag != "" {
		schema.Format = formatTag
	}

	// Handle enum values
	if enumTag != "" {
		enumValues := strings.Split(enumTag, ",")
		schema.Enum = make([]interface{}, len(enumValues))
		for i, val := range enumValues {
			schema.Enum[i] = rm.parseEnumValue(val, goType)
		}
	}

	return schema
}

// parseValidationRules extracts validation constraints from binding tag
func (rm *RouteManager) parseValidationRules(schema *Schema, bindingTag string, goType reflect.Type) {
	if bindingTag == "" {
		return
	}

	rules := strings.Split(bindingTag, ",")
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)

		switch {
		case strings.HasPrefix(rule, "min="):
			if minVal := rm.parseNumericValue(strings.TrimPrefix(rule, "min="), goType); minVal != nil {
				if goType.Kind() == reflect.String {
					schema.MinLength = minVal.(*int)
				} else if fv, ok := minVal.(*float64); ok {
					schema.Minimum = fv
				} else if iv, ok := minVal.(*int64); ok {
					minVal := float64(*iv)
					schema.Minimum = &minVal
				}
			}
		case strings.HasPrefix(rule, "max="):
			if maxVal := rm.parseNumericValue(strings.TrimPrefix(rule, "max="), goType); maxVal != nil {
				if goType.Kind() == reflect.String {
					schema.MaxLength = maxVal.(*int)
				} else if fv, ok := maxVal.(*float64); ok {
					schema.Maximum = fv
				} else if iv, ok := maxVal.(*int64); ok {
					maxVal := float64(*iv)
					schema.Maximum = &maxVal
				}
			}
		case strings.HasPrefix(rule, "len="):
			if lenVal := rm.parseNumericValue(strings.TrimPrefix(rule, "len="), goType); lenVal != nil {
				if goType.Kind() == reflect.String || goType.Kind() == reflect.Slice || goType.Kind() == reflect.Array {
					schema.MinLength = lenVal.(*int)
					schema.MaxLength = lenVal.(*int)
				}
			}
		case strings.HasPrefix(rule, "gt="):
			if gtVal := rm.parseNumericValue(strings.TrimPrefix(rule, "gt="), goType); gtVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					minVal := float64(*gtVal.(*float64)) + 0.000001
					schema.Minimum = &minVal
					schema.ExclusiveMinimum = true
				} else {
					minVal := float64(*gtVal.(*int64)) + 1
					schema.Minimum = &minVal
					schema.ExclusiveMinimum = true
				}
			}
		case strings.HasPrefix(rule, "gte="):
			if gteVal := rm.parseNumericValue(strings.TrimPrefix(rule, "gte="), goType); gteVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					minVal := float64(*gteVal.(*float64))
					schema.Minimum = &minVal
				} else {
					minVal := float64(*gteVal.(*int64))
					schema.Minimum = &minVal
				}
			}
		case strings.HasPrefix(rule, "lt="):
			if ltVal := rm.parseNumericValue(strings.TrimPrefix(rule, "lt="), goType); ltVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					maxVal := float64(*ltVal.(*float64)) - 0.000001
					schema.Maximum = &maxVal
					schema.ExclusiveMaximum = true
				} else {
					maxVal := float64(*ltVal.(*int64)) - 1
					schema.Maximum = &maxVal
					schema.ExclusiveMaximum = true
				}
			}
		case strings.HasPrefix(rule, "lte="):
			if lteVal := rm.parseNumericValue(strings.TrimPrefix(rule, "lte="), goType); lteVal != nil {
				if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
					maxVal := float64(*lteVal.(*float64))
					schema.Maximum = &maxVal
				} else {
					maxVal := float64(*lteVal.(*int64))
					schema.Maximum = &maxVal
				}
			}
		case rule == "email":
			schema.Format = "email"
		case rule == "url":
			schema.Format = "uri"
		case rule == "uri":
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
				schema.Enum[i] = rm.parseEnumValue(val, goType)
			}
		case strings.HasPrefix(rule, "alpha"):
			schema.Pattern = "^[a-zA-Z]+$"
		case strings.HasPrefix(rule, "alphanum"):
			schema.Pattern = "^[a-zA-Z0-9]+$"
		case strings.HasPrefix(rule, "numeric"):
			schema.Pattern = "^[0-9]+$"
		case strings.HasPrefix(rule, "hexadecimal"):
			schema.Pattern = "^[0-9a-fA-F]+$"
		case strings.HasPrefix(rule, "contains="):
			value := strings.TrimPrefix(rule, "contains=")
			if goType.Kind() == reflect.String {
				schema.Pattern = ".*" + value + ".*"
			}
		case strings.HasPrefix(rule, "startswith="):
			value := strings.TrimPrefix(rule, "startswith=")
			if goType.Kind() == reflect.String {
				schema.Pattern = "^" + value + ".*"
			}
		case strings.HasPrefix(rule, "endswith="):
			value := strings.TrimPrefix(rule, "endswith=")
			if goType.Kind() == reflect.String {
				schema.Pattern = ".*" + value + "$"
			}
		}
	}
}

// parseNumericValue parses numeric values from string tags
func (rm *RouteManager) parseNumericValue(value string, goType reflect.Type) interface{} {
	if goType.Kind() == reflect.Float32 || goType.Kind() == reflect.Float64 {
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			return &val
		}
	} else {
		if val, err := strconv.ParseInt(value, 10, 64); err == nil {
			return &val
		}
	}
	return nil
}

// parseExampleValue parses example value based on field type
func (rm *RouteManager) parseExampleValue(value string, goType reflect.Type) interface{} {
	if goType.Kind() == reflect.Ptr {
		goType = goType.Elem()
	}

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

// parseDefaultValue parses default value based on field type
func (rm *RouteManager) parseDefaultValue(value string, goType reflect.Type) interface{} {
	return rm.parseExampleValue(value, goType)
}

// parseEnumValue parses enum value based on field type
func (rm *RouteManager) parseEnumValue(value string, goType reflect.Type) interface{} {
	return rm.parseExampleValue(value, goType)
}

// getSwaggerType converts Go type to Swagger schema
func (rm *RouteManager) getSwaggerType(goType reflect.Type) Schema {
	switch goType.Kind() {
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Schema{Type: "integer", Format: "int64"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer", Format: "int64"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number", Format: "double"}
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		elemType := goType.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		// Get the proper schema for the element type
		// This handles type aliases correctly (e.g., IDESource which is "type IDESource string")
		elemSchema := rm.getSwaggerType(elemType)
		return Schema{
			Type:  "array",
			Items: &elemSchema,
		}
	case reflect.Map:
		// Get key and value types
		valueType := goType.Elem()

		if valueType.Kind() == reflect.Ptr {
			valueType = valueType.Elem()
		}

		// Generate schema for map with additionalProperties
		schema := Schema{Type: "object"}

		// Set value type in additionalProperties
		if valueType.Kind() == reflect.Struct && valueType.String() != "time.Time" {
			// For struct values, use reference
			schema.AdditionalProperties = &Schema{
				Ref: fmt.Sprintf("#/definitions/%s", valueType.Name()),
			}
		} else {
			// For primitive types, get the swagger type
			valueSchema := rm.getSwaggerType(valueType)
			schema.AdditionalProperties = &valueSchema
		}

		return schema
	case reflect.Struct:
		// Handle time.Time specially
		if goType.String() == "time.Time" {
			return Schema{Type: "string", Format: "date-time"}
		}
		// For nested structs, check if it's a known model type
		// If it's a primitive wrapper or basic type, return object
		// Otherwise, we'll handle it in generateSchemaFromModel
		return Schema{Type: "object"}
	case reflect.Ptr:
		return rm.getSwaggerType(goType.Elem())
	case reflect.Interface:
		return Schema{Type: "object"}
	default:
		return Schema{Type: "object"}
	}
}
