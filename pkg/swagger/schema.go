package swagger

import (
	"fmt"
	"reflect"
	"strings"
)

// generateSchemaFromModel generates Swagger schema from a Go struct
func (rm *RouteManager) generateSchemaFromModel(model interface{}) Schema {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// Check if this is a primitive type
	if modelType.Kind() != reflect.Struct {
		return rm.getSwaggerType(modelType)
	}

	// For struct, generate schema with references to nested models
	return rm.generateSchemaWithReferences(model)
}

// generateSchemaWithReferences generates schema using $ref for known nested models
func (rm *RouteManager) generateSchemaWithReferences(model interface{}) Schema {
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldName := field.Name

		// Get all relevant tags
		jsonTag := field.Tag.Get("json")
		bindingTag := field.Tag.Get("binding")
		exampleTag := field.Tag.Get("example")
		defaultTag := field.Tag.Get("default")
		docTag := field.Tag.Get("doc")
		descriptionTag := field.Tag.Get("description")
		formatTag := field.Tag.Get("format")
		enumTag := field.Tag.Get("enum")

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Determine the property name and json options
		propName := fieldName
		isOptional := false
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
			// Check for omitempty and other options
			for _, part := range parts[1:] {
				switch part {
				case "omitempty":
					isOptional = true
				case "stringize":
					// Handle string conversion
				case "omitempty,stringize":
					isOptional = true
				}
			}
		}

		// Generate property schema with type-specific details
		var propSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			// For nested structs, create a reference
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// For anonymous structs, generate inline schema
				collectedModels := make(map[string]interface{})
				propSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// For named structs, use $ref
				propSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			// Handle slices/arrays
			elemType := field.Type.Elem()
			// Check if element is a pointer to struct
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModelName := elemType.Name()
				if nestedModelName == "" {
					// For anonymous struct elements in array
					collectedModels := make(map[string]interface{})
					itemSchema := rm.generateAnonymousStructSchema(elemType, collectedModels)
					propSchema = Schema{
						Type:  "array",
						Items: &itemSchema,
					}
				} else {
					// For named struct elements in array
					propSchema = Schema{
						Type:  "array",
						Items: &Schema{Ref: fmt.Sprintf("#/definitions/%s", nestedModelName)},
					}
				}
			} else {
				propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
			}
		} else {
			propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
		}

		// Set description with priority
		if descriptionTag != "" {
			propSchema.Description = descriptionTag
		} else if docTag != "" {
			propSchema.Description = docTag
		} else {
			propSchema.Description = fmt.Sprintf("Field %s", propName)
		}

		// Set example value
		if exampleTag != "" {
			propSchema.Example = rm.parseExampleValue(exampleTag, field.Type)
		}

		// Set default value
		if defaultTag != "" {
			propSchema.Default = rm.parseDefaultValue(defaultTag, field.Type)
		}

		// Parse binding validation rules
		rm.parseValidationRules(&propSchema, bindingTag, field.Type)

		// Check if field is required (binding tag takes precedence over omitempty)
		if strings.Contains(bindingTag, "required") {
			schema.Required = append(schema.Required, propName)
		} else if !isOptional {
			// Add to required if not explicitly optional
			schema.Required = append(schema.Required, propName)
		}

		schema.Properties[propName] = propSchema
	}

	return schema
}

// generateSchemaFromModelWithDefinitions generates Swagger schema and collects nested models
func (rm *RouteManager) generateSchemaFromModelWithDefinitions(model interface{}, collectedModels map[string]interface{}) Schema {
	modelType := reflect.TypeOf(model)

	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	if modelType.Kind() != reflect.Struct {
		// Handle primitive types
		return rm.getSwaggerType(modelType)
	}

	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		fieldName := field.Name

		// Get all relevant tags
		jsonTag := field.Tag.Get("json")
		bindingTag := field.Tag.Get("binding")
		exampleTag := field.Tag.Get("example")
		defaultTag := field.Tag.Get("default")
		docTag := field.Tag.Get("doc")
		descriptionTag := field.Tag.Get("description")
		formatTag := field.Tag.Get("format")
		enumTag := field.Tag.Get("enum")

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Determine the property name and json options
		propName := fieldName
		isOptional := false
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
			// Check for omitempty and other options
			for _, part := range parts[1:] {
				switch part {
				case "omitempty":
					isOptional = true
				case "stringize":
					// Handle string conversion
				case "omitempty,stringize":
					isOptional = true
				}
			}
		}

		// Generate property schema with type-specific details
		var propSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			// For nested structs, create a reference
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// For anonymous structs, generate inline schema
				propSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// For named structs, collect them and use $ref
				if _, exists := collectedModels[nestedModelName]; !exists {
					// Create a zero value of the struct to collect it
					nestedModel := reflect.Zero(fieldType).Interface()
					collectedModels[nestedModelName] = nestedModel
				}
				propSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			// Handle slices/arrays
			elemType := field.Type.Elem()
			// Check if element is a pointer to struct
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModelName := elemType.Name()
				if nestedModelName == "" {
					// For anonymous struct elements in array
					itemSchema := rm.generateAnonymousStructSchema(elemType, collectedModels)
					propSchema = Schema{
						Type:  "array",
						Items: &itemSchema,
					}
				} else {
					// For named struct elements in array
					if _, exists := collectedModels[nestedModelName]; !exists {
						nestedModel := reflect.Zero(elemType).Interface()
						collectedModels[nestedModelName] = nestedModel
					}
					propSchema = Schema{
						Type:  "array",
						Items: &Schema{Ref: fmt.Sprintf("#/definitions/%s", nestedModelName)},
					}
				}
			} else {
				propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
			}
		} else {
			propSchema = rm.getSwaggerTypeWithDetails(field.Type, bindingTag, formatTag, enumTag)
		}

		// Set description with priority
		if descriptionTag != "" {
			propSchema.Description = descriptionTag
		} else if docTag != "" {
			propSchema.Description = docTag
		} else {
			propSchema.Description = fmt.Sprintf("Field %s", propName)
		}

		// Set example value
		if exampleTag != "" {
			propSchema.Example = rm.parseExampleValue(exampleTag, field.Type)
		}

		// Set default value
		if defaultTag != "" {
			propSchema.Default = rm.parseDefaultValue(defaultTag, field.Type)
		}

		// Parse binding validation rules
		rm.parseValidationRules(&propSchema, bindingTag, field.Type)

		// Check if field is required (binding tag takes precedence over omitempty)
		if strings.Contains(bindingTag, "required") {
			schema.Required = append(schema.Required, propName)
		} else if !isOptional {
			// Add to required if not explicitly optional
			schema.Required = append(schema.Required, propName)
		}

		schema.Properties[propName] = propSchema
	}

	return schema
}

// generateAnonymousStructSchema generates schema for an anonymous struct
func (rm *RouteManager) generateAnonymousStructSchema(structType reflect.Type, collectedModels map[string]interface{}) Schema {
	schema := Schema{
		Type:       "object",
		Properties: make(map[string]Schema),
		Required:   []string{},
	}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Get field name from json tag
		propName := field.Name
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				propName = parts[0]
			}
		}

		// Generate schema for this field
		var fieldSchema Schema
		// Check if this is a nested struct (including pointers to structs)
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			nestedModelName := fieldType.Name()
			if nestedModelName == "" {
				// Anonymous struct inside anonymous struct
				fieldSchema = rm.generateAnonymousStructSchema(fieldType, collectedModels)
			} else {
				// Named struct inside anonymous struct
				if _, exists := collectedModels[nestedModelName]; !exists {
					nestedModel := reflect.Zero(fieldType).Interface()
					collectedModels[nestedModelName] = nestedModel
				}
				fieldSchema = Schema{
					Ref: fmt.Sprintf("#/definitions/%s", nestedModelName),
				}
			}
		} else {
			fieldSchema = rm.getSwaggerType(field.Type)
		}

		// Add description
		descriptionTag := field.Tag.Get("description")
		docTag := field.Tag.Get("doc")
		if descriptionTag != "" {
			fieldSchema.Description = descriptionTag
		} else if docTag != "" {
			fieldSchema.Description = docTag
		} else {
			fieldSchema.Description = fmt.Sprintf("Field %s", propName)
		}

		schema.Properties[propName] = fieldSchema

		// Check if field is required
		if !strings.Contains(jsonTag, "omitempty") {
			schema.Required = append(schema.Required, propName)
		}
	}

	return schema
}

// generateSchemaFromModelWithCache generates schema using cached models to avoid duplication
func (rm *RouteManager) generateSchemaFromModelWithCache(model interface{}, allModels map[string]interface{}) Schema {
	// Get the actual type (handle pointers)
	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// If not a struct, use getSwaggerType (handles gin.H and other non-struct types)
	if modelType.Kind() != reflect.Struct {
		return rm.getSwaggerType(modelType)
	}

	// Otherwise, generate the schema normally
	return rm.generateSchemaWithReferences(model)
}
