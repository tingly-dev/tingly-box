package guardrails

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	"gopkg.in/yaml.v3"
)

//go:embed builtins/*.yaml
var builtinTemplatesFS embed.FS

type builtinTemplateFile struct {
	Policies []guardrailscore.Policy `yaml:"policies"`
}

// LoadBuiltinPolicies loads curated builtin policies from embedded YAML files.
func LoadBuiltinPolicies() ([]guardrailscore.Policy, error) {
	entries, err := fs.ReadDir(builtinTemplatesFS, "builtins")
	if err != nil {
		return nil, fmt.Errorf("read builtin templates: %w", err)
	}

	var policies []guardrailscore.Policy
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
		data, err := builtinTemplatesFS.ReadFile(path.Join("builtins", name))
		if err != nil {
			return nil, fmt.Errorf("read builtin file %s: %w", name, err)
		}
		var file builtinTemplateFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("decode builtin file %s: %w", name, err)
		}
		for i := range file.Policies {
			policy := file.Policies[i]
			if policy.ID == "" {
				return nil, fmt.Errorf("builtin file %s has policy with empty id", name)
			}
			if prev, exists := seen[policy.ID]; exists {
				return nil, fmt.Errorf("duplicate builtin policy id %q in %s and %s", policy.ID, prev, name)
			}
			seen[policy.ID] = name
			if err := validateBuiltinPolicy(name, policy); err != nil {
				return nil, err
			}
			policies = append(policies, policy)
		}
	}

	sort.Slice(policies, func(i, j int) bool {
		if policies[i].Kind == policies[j].Kind {
			if policies[i].Name == policies[j].Name {
				return policies[i].ID < policies[j].ID
			}
			return policies[i].Name < policies[j].Name
		}
		return policies[i].Kind < policies[j].Kind
	})
	return policies, nil
}

func validateBuiltinPolicy(filename string, policy guardrailscore.Policy) error {
	switch policy.Kind {
	case guardrailscore.PolicyKindResourceAccess, guardrailscore.PolicyKindCommandExecution, guardrailscore.PolicyKindContent:
	default:
		return fmt.Errorf("builtin file %s has policy %q with unsupported kind %q", filename, policy.ID, policy.Kind)
	}
	return nil
}
