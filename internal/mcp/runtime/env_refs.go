package runtime

import (
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/envsubst"
)

// EnvRefIssue describes a missing environment reference in MCP runtime config.
type EnvRefIssue struct {
	SourceID  string
	FieldPath string
	VarName   string
}

// ExpandMCPRuntimeEnvRefs expands ${VAR} references in all string fields of MCP runtime config.
// Missing variables are left as-is and returned as issues.
func ExpandMCPRuntimeEnvRefs(cfg *typ.MCPRuntimeConfig) []EnvRefIssue {
	if cfg == nil {
		return nil
	}
	issues := make([]EnvRefIssue, 0)
	for i := range cfg.Sources {
		expandMCPSourceEnvRefs(&cfg.Sources[i], i, &issues)
	}
	return issues
}

// ValidateEnabledMCPSourceEnvRefs checks ${VAR} references for enabled sources only.
// Missing variables are returned as issues.
// In-process transports (e.g. "advisor") are skipped — they never spawn a subprocess
// and do not consume the env map.
func ValidateEnabledMCPSourceEnvRefs(sources []typ.MCPSourceConfig) []EnvRefIssue {
	issues := make([]EnvRefIssue, 0)
	for i := range sources {
		if !typ.IsMCPSourceEnabled(sources[i]) {
			continue
		}
		// Skip in-process transports — they have no subprocess environment requirements.
		if sources[i].Transport == "advisor" {
			continue
		}
		sourceCopy := sources[i]
		expandMCPSourceEnvRefs(&sourceCopy, i, &issues)
	}
	return issues
}

func expandMCPSourceEnvRefs(source *typ.MCPSourceConfig, sourceIndex int, issues *[]EnvRefIssue) {
	if source == nil {
		return
	}
	sourceID := strings.TrimSpace(source.ID)
	if sourceID == "" {
		sourceID = "sources[" + strconv.Itoa(sourceIndex) + "]"
	}
	sourceEnv := make(map[string]string, len(source.Env))
	for k, v := range source.Env {
		sourceEnv[k] = v
	}
	// Expand source env first, then use it as lookup for all other fields.
	for k, v := range sourceEnv {
		expanded, missing := expandStringEnvRefs(v, sourceEnv, true)
		sourceEnv[k] = expanded
		for _, name := range missing {
			*issues = append(*issues, EnvRefIssue{
				SourceID:  sourceID,
				FieldPath: "sources[" + strconv.Itoa(sourceIndex) + "].env." + k,
				VarName:   name,
			})
		}
	}
	source.Env = sourceEnv
	walkExpandValue(reflect.ValueOf(source).Elem(), "sources["+strconv.Itoa(sourceIndex)+"]", sourceID, sourceEnv, issues)
}

func walkExpandValue(v reflect.Value, path, sourceID string, sourceEnv map[string]string, issues *[]EnvRefIssue) {
	if !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return
		}
		walkExpandValue(v.Elem(), path, sourceID, sourceEnv, issues)
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		elem := v.Elem()
		walkExpandValue(elem, path, sourceID, sourceEnv, issues)
		if elem.IsValid() && v.CanSet() {
			v.Set(elem)
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" { // unexported
				continue
			}
			fieldName := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				if parts[0] != "" && parts[0] != "-" {
					fieldName = parts[0]
				}
			}
			walkExpandValue(v.Field(i), path+"."+fieldName, sourceID, sourceEnv, issues)
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			val := iter.Value()
			childPath := path
			if key.Kind() == reflect.String {
				childPath = path + "." + key.String()
			}
			newVal := reflect.New(val.Type()).Elem()
			newVal.Set(val)
			walkExpandValue(newVal, childPath, sourceID, sourceEnv, issues)
			v.SetMapIndex(key, newVal)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			walkExpandValue(v.Index(i), path+"["+strconv.Itoa(i)+"]", sourceID, sourceEnv, issues)
		}
	case reflect.String:
		if !v.CanSet() {
			return
		}
		expanded, missing := expandStringEnvRefs(v.String(), sourceEnv, false)
		v.SetString(expanded)
		for _, name := range missing {
			*issues = append(*issues, EnvRefIssue{
				SourceID:  sourceID,
				FieldPath: path,
				VarName:   name,
			})
		}
	}
}

func expandStringEnvRefs(input string, sourceEnv map[string]string, allowProcessEnv bool) (string, []string) {
	if input == "" || !strings.ContainsRune(input, '$') {
		return input, nil
	}
	// Lookup closure preserving MCP's resolution order:
	//  1. sourceEnv[name], when set, non-empty, and not a self-reference
	//     (e.g. FOO=${FOO} would otherwise expand to itself);
	//  2. the process environment, when allowProcessEnv;
	//  3. otherwise unset — envsubst leaves the literal and reports it missing.
	lookup := func(name string) (string, bool) {
		if val, ok := sourceEnv[name]; ok && val != "" && !isSelfRef(val, name) {
			return val, true
		}
		if allowProcessEnv {
			return os.LookupEnv(name)
		}
		return "", false
	}
	return envsubst.Expand(input, lookup)
}

// isSelfRef reports whether val is a ${name}/$name reference to name itself
// (e.g. name="FOO", val="${FOO}"), which must not be expanded to avoid a loop.
func isSelfRef(val, name string) bool {
	return val == "${"+name+"}" || val == "$"+name
}
