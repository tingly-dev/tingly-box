package services

import (
	"reflect"
	"slices"
	"testing"
)

// Wails binds every exported method of a service except these
// (wails v3 pkg/application/bindings.go internalServiceMethods).
var wailsInternalMethods = []string{"ServiceName", "ServiceStartup", "ServiceShutdown", "ServeHTTP"}

// TestBoundMethods pins what any script in the desktop window can call on
// TinglyService. Adding an exported method makes it callable from the page;
// if that is intended, add it here and to frontend/src/host/desktop.ts
// BOUND_METHODS (whose names desktop.contract.test.ts checks against this
// file). If it is for main's use, keep it unexported or make it a field.
func TestBoundMethods(t *testing.T) {
	var bound []string
	typ := reflect.TypeOf(&TinglyService{})
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; !slices.Contains(wailsInternalMethods, name) {
			bound = append(bound, name)
		}
	}
	want := []string{"GetPort", "GetUserAuthToken", "OpenMainWindow"}
	if !slices.Equal(bound, want) {
		t.Errorf("methods bound for the page = %v, want %v", bound, want)
	}
}
