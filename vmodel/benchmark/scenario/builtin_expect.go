package scenario

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/vmodel"
)

//go:embed builtin_expect.json
var builtinExpectData []byte

type builtinExpectation struct {
	Expect     Expect `json:"expect"`
	Structural Expect `json:"structural"`
}

var builtinExpectations = loadBuiltinExpectations()

func loadBuiltinExpectations() map[string]builtinExpectation {
	var data map[string]builtinExpectation
	if err := vmodel.DecodeConfig(builtinExpectData, &data); err != nil {
		panic(fmt.Errorf("builtin expectations: %w", err))
	}
	for id, entry := range data {
		if err := entry.Expect.Validate(); err != nil {
			panic(fmt.Errorf("%s expect: %w", id, err))
		}
		if err := entry.Structural.Validate(); err != nil {
			panic(fmt.Errorf("%s structural: %w", id, err))
		}
		if len(entry.Expect.Assertions()) == 0 || len(entry.Structural.Assertions()) == 0 {
			panic(fmt.Errorf("%s: missing builtin expectations", id))
		}
	}
	return data
}

func builtinContract(name string) *builtinExpectation {
	entry, ok := builtinExpectations[name]
	if !ok {
		panic(fmt.Errorf("missing builtin expectations %q", name))
	}
	return &entry
}

// BuiltinExpectations returns an independent copy of the file-level contract.
func BuiltinExpectations(name string) (expect, structural *Expect) {
	entry := builtinContract(name)
	raw, err := json.Marshal(entry)
	if err != nil {
		panic(err)
	}
	var copy builtinExpectation
	if err := json.Unmarshal(raw, &copy); err != nil {
		panic(err)
	}
	return &copy.Expect, &copy.Structural
}
