package guardrails

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"path"
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
	log.Println("[DEBUG] LoadBuiltinPolicyTemplates: Starting to load builtin policy templates...")

	// Check if embedded filesystem is accessible
	log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Using embedded FS (embedded files: %v)", builtinTemplatesFS != (embed.FS{}))

	entries, err := fs.ReadDir(builtinTemplatesFS, "builtins")
	if err != nil {
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR reading builtin templates directory: %v", err)
		return nil, fmt.Errorf("read builtin templates: %w", err)
	}
	log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Found %d entries in builtins directory", len(entries))

	var templates []BuiltinPolicyTemplate
	seen := make(map[string]string)
	yamlFileCount := 0
	skippedCount := 0

	for _, entry := range entries {
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Processing entry: %s (isDir: %v)", entry.Name(), entry.IsDir())

		if entry.IsDir() {
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Skipping directory: %s", entry.Name())
			skippedCount++
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: File %s has extension: %s", name, ext)

		if ext != ".yaml" && ext != ".yml" {
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Skipping non-YAML file: %s", name)
			skippedCount++
			continue
		}

		yamlFileCount++
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Reading YAML file: %s", name)

		data, err := builtinTemplatesFS.ReadFile(path.Join("builtins", name))
		if err != nil {
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR reading file %s: %v", name, err)
			return nil, fmt.Errorf("read builtin file %s: %w", name, err)
		}
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Successfully read %d bytes from %s", len(data), name)

		var file builtinTemplateFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR decoding YAML from %s: %v", name, err)
			return nil, fmt.Errorf("decode builtin file %s: %w", name, err)
		}
		log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Successfully decoded %s: found %d templates", name, len(file.Templates))

		for i := range file.Templates {
			tpl := file.Templates[i]
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Processing template [%d]: id=%s, name=%s, kind=%s", i, tpl.ID, tpl.Name, tpl.Kind)

			if tpl.ID == "" {
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR - template with empty id in file %s", name)
				return nil, fmt.Errorf("builtin file %s has template with empty id", name)
			}

			if prev, exists := seen[tpl.ID]; exists {
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR - duplicate template id %q in %s and %s", tpl.ID, prev, name)
				return nil, fmt.Errorf("duplicate builtin template id %q in %s and %s", tpl.ID, prev, name)
			}
			seen[tpl.ID] = name

			if tpl.Kind == "" {
				tpl.Kind = tpl.Policy.Kind
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Template %s: Kind not set, using policy.kind: %s", tpl.ID, tpl.Kind)
			}

			if tpl.Policy.Kind == "" {
				tpl.Policy.Kind = tpl.Kind
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Template %s: Policy.Kind not set, using kind: %s", tpl.ID, tpl.Policy.Kind)
			}

			if tpl.Name == "" {
				tpl.Name = tpl.Policy.Name
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Template %s: Name not set, using policy.name: %s", tpl.ID, tpl.Name)
			}

			if tpl.Policy.Name == "" {
				tpl.Policy.Name = tpl.Name
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Template %s: Policy.Name not set, using name: %s", tpl.ID, tpl.Policy.Name)
			}

			if tpl.Policy.ID == "" {
				tpl.Policy.ID = tpl.ID
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Template %s: Policy.ID not set, using id: %s", tpl.ID, tpl.Policy.ID)
			}

			if err := validateBuiltinTemplate(name, tpl); err != nil {
				log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: ERROR validating template %s: %v", tpl.ID, err)
				return nil, err
			}

			templates = append(templates, tpl)
			log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Successfully added template: id=%s, name=%s", tpl.ID, tpl.Name)
		}
	}

	log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: SUMMARY - Total entries: %d, YAML files: %d, Skipped: %d, Templates loaded: %d",
		len(entries), yamlFileCount, skippedCount, len(templates))

	sort.Slice(templates, func(i, j int) bool {
		if templates[i].Topic == templates[j].Topic {
			return templates[i].Name < templates[j].Name
		}
		return templates[i].Topic < templates[j].Topic
	})

	log.Printf("[DEBUG] LoadBuiltinPolicyTemplates: Completed successfully, returning %d templates", len(templates))
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
