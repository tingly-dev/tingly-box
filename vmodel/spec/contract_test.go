package spec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

func TestCanonicalContractRejectsAlternateShapes(t *testing.T) {
	for _, input := range []string{
		"", "null", "[]", "id: x\ncontent: hello", "name: x\nkind: mock\nmock: {}",
		"id: x\nmock: {}", "id: x\nkind: mock", "id: x\nkind: mock\nmock: null",
		"id: x\nkind: mock\nmock: {}\ncontent: hello",
		"id: x\nkind: sequence\nsteps: [{status: 200}]",
		"id: x\nkind: recording\nmock_responses: {openai_chat: {body: {}}}",
		"id: x\nkind: mock\nmock: {tool_call: {name: run}}",
		"id: x\nkind: mock\nmock: {name: duplicated}",
		"id: x\nkind: mock\nmock: {description: duplicated}",
		"id: x\nkind: mock\nmock: {}\ndelay: 1ms",
		"id: x\nid: y\nkind: mock\nmock: {}",
		`{"id":"x","id":"y","kind":"mock","mock":{}}`,
		"id: x\nkind: mock\nmock: {}\n---\nid: y",
		`{"id":"x","kind":"mock","mock":{}} {"id":"y"}`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := spec.Decode([]byte(input)); err == nil {
				t.Fatalf("accepted alternate or invalid contract: %s", input)
			}
		})
	}
}

func TestMockBehaviorValidation(t *testing.T) {
	for _, block := range []string{
		"delay: -1ms", "delay: fast", "delay: null", "tool_calls: [{arguments: {}}]",
		"usage: {promt_tokens: 1}", "error: {stage: 2}", "error: {status: 200}",
		"tool_calls: [{name: x, raw_arguments: {}, arguments: {}}]",
	} {
		if _, err := spec.Decode([]byte("id: x\nkind: mock\nmock: {" + block + "}")); err == nil {
			t.Fatalf("accepted invalid behavior: %s", block)
		}
	}
}

func TestAllDefinitionFilesUseCanonicalContract(t *testing.T) {
	files, err := filepath.Glob("../testdata/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no definition examples")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			definitions, err := spec.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := spec.New(definitions); err != nil {
				t.Fatal(err)
			}
			for _, format := range []string{"json", "yaml"} {
				exported, err := spec.Export(definitions, format)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := spec.Decode(exported); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	// Obsolete duplicate fixtures must not silently reintroduce another format.
	if _, err := os.Stat("../testdata/legacy"); !os.IsNotExist(err) {
		t.Fatal("legacy definitions directory remains")
	}
}
