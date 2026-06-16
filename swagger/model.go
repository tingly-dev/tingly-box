package swagger

import (
	"reflect"
	"strings"
)

// collectNestedModels recursively collects all nested struct models
func (rm *RouteManager) collectNestedModels(model interface{}, allModels map[string]interface{}, processedModels map[string]bool) {
	modelType := reflect.TypeOf(model)

	// Handle pointer types
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	// Only process struct types (except time.Time)
	if modelType.Kind() != reflect.Struct || modelType.String() == "time.Time" {
		return
	}

	modelName := modelType.Name()

	// Skip anonymous structs (they don't have name)
	if modelName == "" {
		return
	}

	// If already processed, skip
	if processed, exists := processedModels[modelName]; exists && processed {
		return
	}

	// Mark as being processed to avoid infinite recursion
	processedModels[modelName] = true

	// Add to allModels if not already there
	if _, exists := allModels[modelName]; !exists {
		allModels[modelName] = reflect.Zero(modelType).Interface()
	}

	// Recursively process all fields
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Skip non-exported fields
		if field.PkgPath != "" {
			continue
		}

		// Process field type
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		// If field is a struct, recursively collect its nested models
		if fieldType.Kind() == reflect.Struct && fieldType.String() != "time.Time" {
			nestedModel := reflect.Zero(fieldType).Interface()
			rm.collectNestedModels(nestedModel, allModels, processedModels)
		}

		// If field is a slice/array of structs, process the element type
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Array {
			elemType := field.Type.Elem()
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			if elemType.Kind() == reflect.Struct && elemType.String() != "time.Time" {
				nestedModel := reflect.Zero(elemType).Interface()
				rm.collectNestedModels(nestedModel, allModels, processedModels)
			}
		}

		// If field is a map with struct values, process the value type
		if field.Type.Kind() == reflect.Map {
			valueType := field.Type.Elem()
			if valueType.Kind() == reflect.Ptr {
				valueType = valueType.Elem()
			}
			if valueType.Kind() == reflect.Struct && valueType.String() != "time.Time" {
				nestedModel := reflect.Zero(valueType).Interface()
				rm.collectNestedModels(nestedModel, allModels, processedModels)
			}
		}
	}
}

// getModelName extracts the model name from the struct type
func getModelName(model interface{}) string {
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// getModelNameWithFallback extracts model name with fallback to auto-generation for anonymous structs
// Priority: explicitName > struct name > auto-generated from route
func (rm *RouteManager) getModelNameWithFallback(model interface{}, explicitName, method, path, modelType string) string {
	// 1. Use explicit name if provided
	if explicitName != "" {
		return explicitName
	}

	// 2. Try struct name
	t := reflect.TypeOf(model)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if name := t.Name(); name != "" {
		return name
	}

	// 3. Auto-generate from route
	return rm.generateAutoModelName(method, path, modelType)
}

// generateAutoModelName generates a model name from method, path, and model type
// Example: POST /api/v1/users -> PostApiV1UsersRequest or PostApiV1UsersResponse
func (rm *RouteManager) generateAutoModelName(method, path, modelType string) string {
	// Clean path and convert to PascalCase
	// /api/v1/users/:id -> ApiV1UsersId
	parts := strings.Split(path, "/")
	var nameParts []string
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Convert :param to Param
		if strings.HasPrefix(part, ":") {
			part = strings.TrimPrefix(part, ":")
		}
		// Convert to PascalCase (first letter uppercase)
		if len(part) > 0 {
			nameParts = append(nameParts, strings.ToUpper(part[:1])+part[1:])
		}
	}

	// Combine: Method + PathParts + ModelType
	// e.g., Post + ApiV1Users + Request -> PostApiV1UsersRequest
	methodName := strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
	pathName := strings.Join(nameParts, "")

	return methodName + pathName + modelType
}

// generateOperationID generates an operation ID from method and swagger path
// Example: GET /api/v1/auth/validate -> apiV1AuthValidateGet
func (rm *RouteManager) generateOperationID(method, swaggerPath string) string {
	// Convert path to camelCase
	// /api/v1/auth/validate -> apiV1AuthValidate
	parts := strings.Split(swaggerPath, "/")
	var nameParts []string
	for i, part := range parts {
		if part == "" {
			continue
		}
		// Remove braces from path params: {id} -> Id
		part = strings.Trim(part, "{}")
		if len(part) == 0 {
			continue
		}
		// First part lowercase, rest PascalCase
		if i == 1 { // First non-empty part after leading /
			nameParts = append(nameParts, strings.ToLower(part[:1])+part[1:])
		} else {
			nameParts = append(nameParts, strings.ToUpper(part[:1])+part[1:])
		}
	}

	// Combine: pathName + Method (lowercase)
	// e.g., apiV1AuthValidate + get -> apiV1AuthValidateGet
	pathName := strings.Join(nameParts, "")
	methodLower := strings.ToLower(method)

	return pathName + strings.ToUpper(methodLower[:1]) + methodLower[1:]
}
