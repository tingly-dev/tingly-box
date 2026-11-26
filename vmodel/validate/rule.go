// Package validate provides protocol-independent matching over JSON observations.
package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// Rule compares a selected value with a literal or another value in the same
// observation. Paths use GJSON syntax. An empty path selects the whole value.
// Value is raw JSON to distinguish omitted values from explicit null.
type Rule struct {
	Name  string          `json:"name,omitempty"`
	Path  string          `json:"path"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value,omitempty"`
	Ref   *string         `json:"ref,omitempty"`
}

func (r Rule) Validate() error {
	switch r.Op {
	case "exists", "absent":
		if r.Value != nil || r.Ref != nil {
			return fmt.Errorf("%s takes no operand", r.Op)
		}
		return nil
	case "eq", "ne", "contains", "regex", "one_of", "gte":
	default:
		return fmt.Errorf("unknown match operator %q", r.Op)
	}
	if (r.Value == nil) == (r.Ref == nil) {
		return fmt.Errorf("%s requires exactly one value or ref", r.Op)
	}
	if r.Ref != nil {
		return nil
	}
	v, err := decode(r.Value)
	if err != nil {
		return err
	}
	return operand(r.Op, v)
}

func decode(raw []byte) (any, error) {
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	err := d.Decode(&v)
	return v, err
}

func operand(op string, v any) error {
	switch op {
	case "regex":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("regex requires a string")
		}
		_, err := regexp.Compile(s)
		return err
	case "one_of":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("one_of requires an array")
		}
	case "gte":
		if _, ok := v.(json.Number); !ok {
			return fmt.Errorf("gte requires a number")
		}
	}
	return nil
}

func selectValue(raw []byte, path string) (any, bool, error) {
	if path == "" {
		v, err := decode(raw)
		return v, true, err
	}
	v := gjson.GetBytes(raw, path)
	if !v.Exists() {
		return nil, false, nil
	}
	x, err := decode([]byte(v.Raw))
	return x, true, err
}

// Match evaluates the rule without coercing types or silently accepting missing fields.
func (r Rule) Match(raw []byte) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if !json.Valid(raw) {
		return fmt.Errorf("observation is not JSON")
	}
	got, exists, err := selectValue(raw, r.Path)
	if err != nil {
		return err
	}
	if r.Op == "absent" && !exists || r.Op == "exists" && exists {
		return nil
	}
	if !exists {
		return fmt.Errorf("missing field %q", r.Path)
	}
	if r.Op == "absent" {
		return fmt.Errorf("unexpected field %q", r.Path)
	}
	var want any
	if r.Ref != nil {
		var ok bool
		want, ok, err = selectValue(raw, *r.Ref)
		if !ok {
			return fmt.Errorf("missing reference %q", *r.Ref)
		}
	} else {
		want, err = decode(r.Value)
	}
	if err != nil {
		return err
	}
	if err := operand(r.Op, want); err != nil {
		return err
	}
	if matches(r.Op, got, want) {
		return nil
	}
	return fmt.Errorf("%s: got %v, want %s %v", r.Path, got, r.Op, want)
}

func matches(op string, got, want any) bool {
	switch op {
	case "eq":
		return reflect.DeepEqual(got, want)
	case "ne":
		return !reflect.DeepEqual(got, want)
	case "contains":
		switch g := got.(type) {
		case string:
			w, ok := want.(string)
			return ok && Contains(g, w)
		case []any:
			for _, x := range g {
				if reflect.DeepEqual(x, want) {
					return true
				}
			}
		}
	case "regex":
		g, ok := got.(string)
		if ok {
			matched, _ := regexp.MatchString(want.(string), g)
			return matched
		}
	case "one_of":
		for _, x := range want.([]any) {
			if reflect.DeepEqual(x, got) {
				return true
			}
		}
	case "gte":
		g, ok := got.(json.Number)
		if ok {
			a, e := new(big.Rat).SetString(string(g))
			b, f := new(big.Rat).SetString(string(want.(json.Number)))
			return e && f && a.Cmp(b) >= 0
		}
	}
	return false
}

// Contains is the common literal text matcher (no trimming or case folding).
func Contains(actual, expected string) bool { return strings.Contains(actual, expected) }

// EqualJSON compares complete JSON values without ignoring fields or array order.
func EqualJSON(actual, expected []byte) error { return (Rule{Op: "eq", Value: expected}).Match(actual) }

// Failure is a named validation failure shared by typed adapters.
type Failure struct {
	Name string
	Err  error
}

func (f Failure) Error() string { return fmt.Sprintf("%s: %v", f.Name, f.Err) }

// Evaluate runs all named checks and retains every failure in input order.
func Evaluate[T any](actual T, checks []NamedCheck[T]) []Failure {
	var failures []Failure
	for _, c := range checks {
		if err := c.Check(actual); err != nil {
			failures = append(failures, Failure{c.Name, err})
		}
	}
	return failures
}

type NamedCheck[T any] struct {
	Name  string
	Check func(T) error
}

// Value matches a literal against an in-memory value using the same operators.
func Value(op string, actual, expected any) error {
	a, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	b, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	return (Rule{Op: op, Value: b}).Match(a)
}
