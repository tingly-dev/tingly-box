package swagger

import (
	"reflect"
	"strings"
)

// getModelNameWithFallback extracts model name with fallback to auto-generation for anonymous structs
// Priority: explicitName > struct name > auto-generated from route
func (rm *RouteManager) getModelNameWithFallback(model interface{}, explicitName, method, path, modelType string) string {
	// 1. Use explicit name if provided
	if explicitName != "" {
		return explicitName
	}

	// 2. Try struct name
	if name := derefType(reflect.TypeOf(model)).Name(); name != "" {
		return name
	}

	// 3. Auto-generate from route
	return generateAutoModelName(method, path, modelType)
}

// generateAutoModelName generates a model name from method, path, and model type
// Example: POST /api/v1/users -> PostApiV1UsersRequest or PostApiV1UsersResponse
func generateAutoModelName(method, path, modelType string) string {
	// Clean path and convert to PascalCase: /api/v1/users/:id -> ApiV1UsersId
	var nameParts []string
	for _, part := range strings.Split(path, "/") {
		part = strings.TrimPrefix(part, ":")
		if part == "" {
			continue
		}
		nameParts = append(nameParts, strings.ToUpper(part[:1])+part[1:])
	}

	// Combine: Method + PathParts + ModelType
	// e.g., Post + ApiV1Users + Request -> PostApiV1UsersRequest
	methodName := strings.ToUpper(method[:1]) + strings.ToLower(method[1:])

	return methodName + strings.Join(nameParts, "") + modelType
}

// generateOperationID generates an operation ID from method and swagger path
// Example: GET /api/v1/auth/validate -> apiV1AuthValidateGet
func generateOperationID(method, swaggerPath string) string {
	// Convert path to camelCase: /api/v1/auth/validate -> apiV1AuthValidate
	var nameParts []string
	for _, part := range strings.Split(swaggerPath, "/") {
		// Remove braces from path params: {id} -> Id
		part = strings.Trim(part, "{}")
		if part == "" {
			continue
		}
		// First part lowercase, rest PascalCase
		if len(nameParts) == 0 {
			nameParts = append(nameParts, strings.ToLower(part[:1])+part[1:])
		} else {
			nameParts = append(nameParts, strings.ToUpper(part[:1])+part[1:])
		}
	}

	// Combine: pathName + Method: apiV1AuthValidate + get -> apiV1AuthValidateGet
	methodLower := strings.ToLower(method)
	return strings.Join(nameParts, "") + strings.ToUpper(methodLower[:1]) + methodLower[1:]
}
