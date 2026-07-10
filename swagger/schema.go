package swagger

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

var (
	timeType       = reflect.TypeOf(time.Time{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
)

// getRefPrefix returns the reference prefix based on version
func getRefPrefix(version Version) string {
	switch version {
	case VersionV3:
		return "#/components/schemas/"
	default:
		return "#/definitions/"
	}
}

// schemaGen generates version-aware schemas from Go types.
// Named struct types encountered while generating are recorded in collected so
// the spec builders can emit a definition for every referenced model.
type schemaGen struct {
	refPrefix string
	collected map[string]reflect.Type
}

func newSchemaGen(version Version) *schemaGen {
	return &schemaGen{
		refPrefix: getRefPrefix(version),
		collected: make(map[string]reflect.Type),
	}
}

// buildDefinitions generates schemas for every registered model plus all
// named struct types reachable from them (nested structs, slice elements,
// map values, embedded fields).
func (g *schemaGen) buildDefinitions(modelSet map[string]interface{}) map[string]Schema {
	definitions := make(map[string]Schema, len(modelSet))
	for name, model := range modelSet {
		definitions[name] = g.modelSchema(model)
	}

	// Generating a schema may discover new nested models; iterate until no
	// undiscovered model remains. Registration into collected happens at most
	// once per name, so this terminates even with cyclic models.
	for {
		var pending []string
		for name := range g.collected {
			if _, done := definitions[name]; !done {
				pending = append(pending, name)
			}
		}
		if len(pending) == 0 {
			return definitions
		}
		for _, name := range pending {
			definitions[name] = g.structSchema(g.collected[name])
		}
	}
}

// modelSchema builds the expanded schema for a model that is emitted as a
// definition. Unlike typeSchema, a named struct is expanded here rather than
// turned into a $ref.
func (g *schemaGen) modelSchema(model interface{}) Schema {
	t := derefType(reflect.TypeOf(model))
	if t.Kind() != reflect.Struct || t == timeType {
		return g.typeSchema(t)
	}
	return g.structSchema(t)
}

// structSchema builds an object schema from the exported fields of a struct.
func (g *schemaGen) structSchema(t reflect.Type) Schema {
	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}
	g.addStructFields(&schema, t)
	return schema
}

// addStructFields adds property schemas for the fields of t to schema.
// Embedded structs without an explicit json name are flattened, matching
// encoding/json marshaling behavior.
func (g *schemaGen) addStructFields(schema *Schema, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip non-exported fields (embedded types are exported when the type
		// name is; PkgPath is empty for them)
		if field.PkgPath != "" {
			continue
		}

		jsonTag := field.Tag.Get("json")
		if tagName(jsonTag) == "-" {
			continue
		}

		if field.Anonymous && tagName(jsonTag) == "" {
			embeddedType := derefType(field.Type)
			if embeddedType.Kind() == reflect.Struct && embeddedType != timeType {
				g.addStructFields(schema, embeddedType)
				continue
			}
		}

		propName := field.Name
		if name := tagName(jsonTag); name != "" {
			propName = name
		}

		schema.Properties[propName] = g.fieldSchema(field, propName)

		// A field is required when binding says so, or when it is not marked
		// omitempty (it always appears in the marshaled JSON).
		bindingTag := field.Tag.Get("binding")
		if strings.Contains(bindingTag, "required") || !hasJSONOption(jsonTag, "omitempty") {
			schema.Required = append(schema.Required, propName)
		}
	}
}

// fieldSchema builds the property schema for a single struct field, applying
// tag-driven details (description, example, default, format, enum, binding
// validation rules).
func (g *schemaGen) fieldSchema(field reflect.StructField, propName string) Schema {
	propSchema := g.typeSchema(field.Type)

	// $ref must not carry sibling properties (OpenAPI 3.0 forbids them)
	if propSchema.Ref != "" {
		return propSchema
	}

	if format := field.Tag.Get("format"); format != "" {
		propSchema.Format = format
	}

	if enumTag := field.Tag.Get("enum"); enumTag != "" {
		enumValues := strings.Split(enumTag, ",")
		propSchema.Enum = make([]interface{}, len(enumValues))
		for i, val := range enumValues {
			propSchema.Enum[i] = parseTaggedValue(val, field.Type)
		}
	}

	// Description priority: description > doc > generated fallback
	if desc := field.Tag.Get("description"); desc != "" {
		propSchema.Description = desc
	} else if doc := field.Tag.Get("doc"); doc != "" {
		propSchema.Description = doc
	} else {
		propSchema.Description = fmt.Sprintf("Field %s", propName)
	}

	if example := field.Tag.Get("example"); example != "" {
		propSchema.Example = parseTaggedValue(example, field.Type)
	}

	if defaultTag := field.Tag.Get("default"); defaultTag != "" {
		propSchema.Default = parseTaggedValue(defaultTag, field.Type)
	}

	parseValidationRules(&propSchema, field.Tag.Get("binding"), derefType(field.Type))

	return propSchema
}

// typeSchema returns the schema for a Go type as it appears inside another
// schema. Named structs become a $ref and are queued for definition emission;
// anonymous structs are inlined.
func (g *schemaGen) typeSchema(t reflect.Type) Schema {
	t = derefType(t)

	switch t.Kind() {
	case reflect.String:
		return Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return Schema{Type: "integer", Format: "int64"}
	case reflect.Float32, reflect.Float64:
		return Schema{Type: "number", Format: "double"}
	case reflect.Bool:
		return Schema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		// json.RawMessage marshals as arbitrary JSON: any-typed schema
		if t == rawMessageType {
			return Schema{}
		}
		// []byte marshals as a base64 string
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			return Schema{Type: "string", Format: "byte"}
		}
		elemSchema := g.typeSchema(t.Elem())
		return Schema{Type: "array", Items: &elemSchema}
	case reflect.Map:
		valueSchema := g.typeSchema(t.Elem())
		return Schema{Type: "object", AdditionalProperties: &valueSchema}
	case reflect.Struct:
		if t == timeType {
			return Schema{Type: "string", Format: "date-time"}
		}
		if t.Name() == "" {
			// Anonymous struct: inline the schema
			return g.structSchema(t)
		}
		g.collect(t)
		return Schema{Ref: g.refPrefix + t.Name()}
	case reflect.Interface:
		return Schema{Type: "object"}
	default:
		return Schema{Type: "object"}
	}
}

// collect records a named struct type so buildDefinitions emits it.
func (g *schemaGen) collect(t reflect.Type) {
	name := t.Name()
	if name == "" {
		return
	}
	if _, exists := g.collected[name]; !exists {
		g.collected[name] = t
	}
}

// derefType unwraps pointer types.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// tagName returns the name part of a json/form struct tag ("-" included).
func tagName(tag string) string {
	if tag == "" {
		return ""
	}
	if idx := strings.Index(tag, ","); idx >= 0 {
		return tag[:idx]
	}
	return tag
}

// hasJSONOption reports whether a struct tag carries the given option
// (e.g. "omitempty" in `json:"name,omitempty"`).
func hasJSONOption(tag, option string) bool {
	parts := strings.Split(tag, ",")
	for _, part := range parts[1:] {
		if part == option {
			return true
		}
	}
	return false
}
