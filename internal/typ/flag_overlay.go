package typ

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
)

// ProbeFlagsHeader carries a per-request rule-flag overlay from the probe
// layer into TB's own loopback handler. The value is base64url (unpadded) of
// a JSON object keyed by registry flag keys. It belongs to the same admin-only
// probe header family as X-Tingly-Probe-Rule / X-Tingly-Probe-Service and is
// only honoured on the loopback path (see ResolveRuleFlagsWithScenario).
const ProbeFlagsHeader = "X-Tingly-Probe-Flags"

// FlagOverlay is a partial rule-flag set: only the keys present are meant to
// be applied, each replacing the resolved (rule + scenario inherited) value.
// Raw JSON keeps the "present vs absent" distinction a typed RuleFlags cannot
// express (a zero value and an unset field look the same there), which is the
// whole point of an overlay — "turn this scenario-default flag OFF for one
// request" must be representable.
type FlagOverlay map[string]json.RawMessage

// Keys returns the overlay's keys in sorted order, for deterministic
// validation errors, encoding and logging.
func (o FlagOverlay) Keys() []string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flagSpecByKey looks a flag up in the registry (the single source of truth
// for which keys exist and what type they carry).
func flagSpecByKey(key string) (FlagSpec, bool) {
	for _, spec := range RuleFlagRegistry() {
		if spec.Key == key {
			return spec, true
		}
	}
	return FlagSpec{}, false
}

// ValidateFlagOverlay rejects unknown keys and values whose JSON type does
// not match the registry spec, so a typo or a wrong type fails at the API
// boundary (400) instead of being silently dropped inside the handler.
func ValidateFlagOverlay(overlay FlagOverlay) error {
	for _, key := range overlay.Keys() {
		spec, ok := flagSpecByKey(key)
		if !ok {
			return fmt.Errorf("unknown flag %q", key)
		}
		if err := validateFlagValue(spec, overlay[key]); err != nil {
			return fmt.Errorf("flag %q: %w", key, err)
		}
	}
	return nil
}

func validateFlagValue(spec FlagSpec, raw json.RawMessage) error {
	switch spec.Type {
	case FlagTypeBool:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return fmt.Errorf("expected a boolean")
		}
	case FlagTypeString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("expected a string")
		}
	case FlagTypeInt:
		var n int
		if err := json.Unmarshal(raw, &n); err != nil || n < 0 {
			return fmt.Errorf("expected a non-negative integer")
		}
	case FlagTypeEnum:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("expected a string")
		}
		if s == "" {
			return nil // empty = inactive / default, always legal
		}
		for _, opt := range spec.Options {
			if opt.Value == s {
				return nil
			}
		}
		return fmt.Errorf("value %q is not one of the allowed options", s)
	case FlagTypeServiceRef:
		var ref VisionProxyService
		if err := json.Unmarshal(raw, &ref); err != nil {
			return fmt.Errorf("expected a {provider, model} object")
		}
	case FlagTypeHeaders:
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("expected a name→value object")
		}
	default:
		return fmt.Errorf("unsupported flag type %q", spec.Type)
	}
	return nil
}

// ApplyFlagOverlay returns flags with every key present in overlay replaced
// by the overlay's value; keys absent from the overlay are untouched. The
// merge goes through the JSON form so it follows the same field names the
// registry and the wire use, and an explicit zero value ("false", "") in the
// overlay really does clear the field.
func ApplyFlagOverlay(flags RuleFlags, overlay FlagOverlay) (RuleFlags, error) {
	if len(overlay) == 0 {
		return flags, nil
	}
	if err := ValidateFlagOverlay(overlay); err != nil {
		return flags, err
	}
	base, err := json.Marshal(flags)
	if err != nil {
		return flags, fmt.Errorf("marshal flags: %w", err)
	}
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(base, &merged); err != nil {
		return flags, fmt.Errorf("unmarshal flags: %w", err)
	}
	for k, v := range overlay {
		merged[k] = v
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return flags, fmt.Errorf("marshal merged flags: %w", err)
	}
	var result RuleFlags
	if err := json.Unmarshal(out, &result); err != nil {
		return flags, fmt.Errorf("unmarshal merged flags: %w", err)
	}
	return result, nil
}

// EncodeFlagOverlay renders an overlay as the ProbeFlagsHeader value.
func EncodeFlagOverlay(overlay FlagOverlay) (string, error) {
	if len(overlay) == 0 {
		return "", nil
	}
	b, err := json.Marshal(overlay)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeFlagOverlay parses a ProbeFlagsHeader value. An empty value decodes
// to an empty overlay.
func DecodeFlagOverlay(value string) (FlagOverlay, error) {
	if value == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", ProbeFlagsHeader, err)
	}
	var overlay FlagOverlay
	if err := json.Unmarshal(b, &overlay); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ProbeFlagsHeader, err)
	}
	return overlay, nil
}
