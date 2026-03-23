package guardrails

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtins/*.yaml
var builtinTemplatesFS embed.FS

// BuiltinPolicyTemplate is a curated starter policy shown in the Builtins page.
type BuiltinPolicyTemplate struct {
	ID          string     `json:"id" yaml:"id"`
	Name        string     `json:"name" yaml:"name"`
	Summary     string     `json:"summary,omitempty" yaml:"summary,omitempty"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Kind        PolicyKind `json:"kind" yaml:"kind"`
	Topic       string     `json:"topic,omitempty" yaml:"topic,omitempty"`
	Tags        []string   `json:"tags,omitempty" yaml:"tags,omitempty"`
	Policy      Policy     `json:"policy" yaml:"policy"`
}

type builtinTemplateFile struct {
	Templates []BuiltinPolicyTemplate `yaml:"templates"`
}

// builtinTopics defines the controlled topical taxonomy used by the Builtins page.
var builtinTopics = map[string]struct{}{
	"filesystem_access": {},
	"command_execution": {},
	"output_filtering":  {},
}

// LoadBuiltinPolicyTemplates loads curated builtin policy templates from embedded YAML files.
func LoadBuiltinPolicyTemplates() ([]BuiltinPolicyTemplate, error) {
	entries, err := fs.ReadDir(builtinTemplatesFS, "builtins")
	if err != nil {
		return nil, fmt.Errorf("read builtin templates: %w", err)
	}

	var templates []BuiltinPolicyTemplate
	seen := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := builtinTemplatesFS.ReadFile(filepath.Join("builtins", name))
		if err != nil {
			return nil, fmt.Errorf("read builtin file %s: %w", name, err)
		}
		var file builtinTemplateFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("decode builtin file %s: %w", name, err)
		}
		for i := range file.Templates {
			tpl := file.Templates[i]
			if tpl.ID == "" {
				return nil, fmt.Errorf("builtin file %s has template with empty id", name)
			}
			if prev, exists := seen[tpl.ID]; exists {
				return nil, fmt.Errorf("duplicate builtin template id %q in %s and %s", tpl.ID, prev, name)
			}
			seen[tpl.ID] = name
			if tpl.Kind == "" {
				tpl.Kind = tpl.Policy.Kind
			}
			if tpl.Policy.Kind == "" {
				tpl.Policy.Kind = tpl.Kind
			}
			if tpl.Name == "" {
				tpl.Name = tpl.Policy.Name
			}
			if tpl.Policy.Name == "" {
				tpl.Policy.Name = tpl.Name
			}
			if tpl.Policy.ID == "" {
				tpl.Policy.ID = tpl.ID
			}
			if err := validateBuiltinTemplate(name, tpl); err != nil {
				return nil, err
			}
			templates = append(templates, tpl)
		}
	}

	sort.Slice(templates, func(i, j int) bool {
		if templates[i].Topic == templates[j].Topic {
			return templates[i].Name < templates[j].Name
		}
		return templates[i].Topic < templates[j].Topic
	})
	return templates, nil
}

func validateBuiltinTemplate(filename string, tpl BuiltinPolicyTemplate) error {
	switch tpl.Kind {
	case PolicyKindResourceAccess, PolicyKindCommandExecution, PolicyKindContent:
	default:
		return fmt.Errorf("builtin file %s has template %q with unsupported kind %q", filename, tpl.ID, tpl.Kind)
	}

	// Topic drives UI grouping and filtering, so keep it on a small controlled set.
	if tpl.Topic != "" {
		if _, ok := builtinTopics[tpl.Topic]; !ok {
			return fmt.Errorf("builtin file %s has template %q with unsupported topic %q", filename, tpl.ID, tpl.Topic)
		}
	}

	if tpl.Policy.Kind != tpl.Kind {
		return fmt.Errorf("builtin file %s has template %q with mismatched policy kind %q", filename, tpl.ID, tpl.Policy.Kind)
	}

	return nil
}
