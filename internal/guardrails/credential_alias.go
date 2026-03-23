package guardrails

import (
	"sort"
	"strings"
)

// CredentialMaskState tracks the credential substitutions observed in one request flow.
// Keep this request-scoped; restore should only operate on aliases introduced locally.
type CredentialMaskState struct {
	AliasToReal map[string]string `json:"alias_to_real,omitempty"`
	RealToAlias map[string]string `json:"real_to_alias,omitempty"`
	UsedRefs    []string          `json:"used_refs,omitempty"`
}

// CredentialMaskStateContextKey is the shared gin-context key used to carry
// request-scoped alias mappings from request masking to stream/tool restoration.
const CredentialMaskStateContextKey = "guardrails_credential_mask_state"

func NewCredentialMaskState() *CredentialMaskState {
	return &CredentialMaskState{
		AliasToReal: make(map[string]string),
		RealToAlias: make(map[string]string),
	}
}

func MayContainAliasToken(text string) bool {
	return text != "" && strings.Contains(text, AliasTokenPrefix)
}

func (s *CredentialMaskState) remember(credential ProtectedCredential) {
	if s == nil || credential.ID == "" || credential.Secret == "" || credential.AliasToken == "" {
		return
	}
	s.AliasToReal[credential.AliasToken] = credential.Secret
	s.RealToAlias[credential.Secret] = credential.AliasToken
	for _, id := range s.UsedRefs {
		if id == credential.ID {
			return
		}
	}
	s.UsedRefs = append(s.UsedRefs, credential.ID)
}

func AliasText(text string, credentials []ProtectedCredential, state *CredentialMaskState) (string, bool) {
	if text == "" || len(credentials) == 0 {
		return text, false
	}
	replaced := text
	changed := false
	for _, credential := range credentials {
		if credential.Secret == "" || credential.AliasToken == "" {
			continue
		}
		if strings.Contains(replaced, credential.Secret) {
			replaced = strings.ReplaceAll(replaced, credential.Secret, credential.AliasToken)
			changed = true
			if state != nil {
				state.remember(credential)
			}
		}
	}
	return replaced, changed
}

func RestoreText(text string, state *CredentialMaskState) (string, bool) {
	if text == "" || state == nil || len(state.AliasToReal) == 0 || !MayContainAliasToken(text) {
		return text, false
	}
	aliases := make([]string, 0, len(state.AliasToReal))
	for alias := range state.AliasToReal {
		aliases = append(aliases, alias)
	}
	sort.Slice(aliases, func(i, j int) bool {
		return len(aliases[i]) > len(aliases[j])
	})
	replaced := text
	changed := false
	for _, alias := range aliases {
		real := state.AliasToReal[alias]
		if alias == "" || real == "" {
			continue
		}
		if strings.Contains(replaced, alias) {
			replaced = strings.ReplaceAll(replaced, alias, real)
			changed = true
		}
	}
	return replaced, changed
}

func AliasContent(content Content, credentials []ProtectedCredential, state *CredentialMaskState) (Content, bool) {
	changed := false
	cloned := content
	if cloned.Text != "" {
		if next, ok := AliasText(cloned.Text, credentials, state); ok {
			cloned.Text = next
			changed = true
		}
	}
	if len(cloned.Messages) > 0 {
		messages := append([]Message(nil), cloned.Messages...)
		for i := range messages {
			if next, ok := AliasText(messages[i].Content, credentials, state); ok {
				messages[i].Content = next
				changed = true
			}
		}
		cloned.Messages = messages
	}
	if cloned.Command != nil {
		command := *cloned.Command
		if nextArgs, ok := aliasCommandArgs(command.Arguments, credentials, state); ok {
			command.Arguments = nextArgs
			command.AttachDerivedFields()
			changed = true
		}
		cloned.Command = &command
	}
	return cloned, changed
}

func AliasStructuredValue(value interface{}, credentials []ProtectedCredential, state *CredentialMaskState) (interface{}, bool) {
	return aliasValue(value, credentials, state)
}

func RestoreContent(content Content, state *CredentialMaskState) (Content, bool) {
	changed := false
	cloned := content
	if cloned.Text != "" {
		if next, ok := RestoreText(cloned.Text, state); ok {
			cloned.Text = next
			changed = true
		}
	}
	if len(cloned.Messages) > 0 {
		messages := append([]Message(nil), cloned.Messages...)
		for i := range messages {
			if next, ok := RestoreText(messages[i].Content, state); ok {
				messages[i].Content = next
				changed = true
			}
		}
		cloned.Messages = messages
	}
	if cloned.Command != nil {
		command := *cloned.Command
		if nextArgs, ok := restoreCommandArgs(command.Arguments, state); ok {
			command.Arguments = nextArgs
			command.AttachDerivedFields()
			changed = true
		}
		cloned.Command = &command
	}
	return cloned, changed
}

func RestoreStructuredValue(value interface{}, state *CredentialMaskState) (interface{}, bool) {
	return restoreValue(value, state)
}

func aliasCommandArgs(arguments map[string]interface{}, credentials []ProtectedCredential, state *CredentialMaskState) (map[string]interface{}, bool) {
	if len(arguments) == 0 {
		return arguments, false
	}
	next, changed := aliasValue(arguments, credentials, state)
	if !changed {
		return arguments, false
	}
	mapped, _ := next.(map[string]interface{})
	return mapped, true
}

func restoreCommandArgs(arguments map[string]interface{}, state *CredentialMaskState) (map[string]interface{}, bool) {
	if len(arguments) == 0 {
		return arguments, false
	}
	next, changed := restoreValue(arguments, state)
	if !changed {
		return arguments, false
	}
	mapped, _ := next.(map[string]interface{})
	return mapped, true
}

func aliasValue(value interface{}, credentials []ProtectedCredential, state *CredentialMaskState) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		return AliasText(typed, credentials, state)
	case []interface{}:
		next := make([]interface{}, len(typed))
		changed := false
		for i := range typed {
			replaced, ok := aliasValue(typed[i], credentials, state)
			next[i] = replaced
			changed = changed || ok
		}
		if !changed {
			return value, false
		}
		return next, true
	case map[string]interface{}:
		next := make(map[string]interface{}, len(typed))
		changed := false
		for key, item := range typed {
			replaced, ok := aliasValue(item, credentials, state)
			next[key] = replaced
			changed = changed || ok
		}
		if !changed {
			return value, false
		}
		return next, true
	default:
		return value, false
	}
}

func restoreValue(value interface{}, state *CredentialMaskState) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		return RestoreText(typed, state)
	case []interface{}:
		next := make([]interface{}, len(typed))
		changed := false
		for i := range typed {
			replaced, ok := restoreValue(typed[i], state)
			next[i] = replaced
			changed = changed || ok
		}
		if !changed {
			return value, false
		}
		return next, true
	case map[string]interface{}:
		next := make(map[string]interface{}, len(typed))
		changed := false
		for key, item := range typed {
			replaced, ok := restoreValue(item, state)
			next[key] = replaced
			changed = changed || ok
		}
		if !changed {
			return value, false
		}
		return next, true
	default:
		return value, false
	}
}
