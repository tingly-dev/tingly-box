// Package live runs Projects journeys for real: the real tingly-project
// server and the real Claude Code CLI it launches, against a scripted model
// served by `tingly-box vmodel` (a kind: script definition, see
// .design/vmodel-script.md). A journey file says how the project starts,
// what the model answers in each conversation, what the user does, and what
// Projects must show at the end. It complements the in-process harness
// (package harness): that one scripts the agents, this one scripts only the
// model and keeps everything else real.
package live

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Journey is one journey file.
type Journey struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Project     ProjectSpec `yaml:"project"`
	// Model is a vmodel definition of kind script, handed to
	// `tingly-box vmodel` as is.
	Model map[string]any `yaml:"model"`
	Steps []Action       `yaml:"steps"`
	// Timeout bounds each step that waits (default 60s).
	Timeout string `yaml:"timeout"`
	Expect  Expect `yaml:"expect"`
}

// ProjectSpec is the project the journey starts with, on a fresh repository.
type ProjectSpec struct {
	Name         string            `yaml:"name"`
	Goal         string            `yaml:"goal"`
	Instructions string            `yaml:"instructions"`
	Files        map[string]string `yaml:"files"`
}

// Action is one thing the user (or the driver) does; exactly one is set.
type Action struct {
	// Say writes in the project conversation.
	Say string `yaml:"say"`
	// Message writes to a thread.
	Message *MessageAction `yaml:"message"`
	// Approve and Deny answer the next approval card of a thread.
	Approve *ApprovalAction `yaml:"approve"`
	Deny    *ApprovalAction `yaml:"deny"`
	// Wait waits for a thread status.
	Wait *WaitAction `yaml:"wait"`
	// Served waits until the model side answered a step ("track.step").
	Served string `yaml:"served"`
	// Reached waits until the step's request arrived; with after: on that
	// step its reply is still held, so the next actions happen while the
	// client waits for it.
	Reached string `yaml:"reached"`
	// Signal releases model steps held with after: [signal:<name>].
	Signal string `yaml:"signal"`
	// StartSuggestions starts threads from the coordinator's newest
	// suggestions card (all of them when Indexes is empty).
	StartSuggestions *SuggestionsAction `yaml:"start_suggestions"`
	// Choose picks an option of the coordinator's newest open decision card.
	Choose *ChooseAction `yaml:"choose"`
	// Settle waits until the coordinator and every thread are idle: a step
	// served is not yet a turn ended, and a message sent in between joins
	// the ending turn.
	Settle bool `yaml:"settle"`
}

type SuggestionsAction struct {
	Indexes []int `yaml:"indexes"`
}

type ChooseAction struct {
	Option int `yaml:"option"`
}

type MessageAction struct {
	Thread  string `yaml:"thread"`
	Content string `yaml:"content"`
	// Mode is insert (the default) or next_turn.
	Mode string `yaml:"mode"`
}

type ApprovalAction struct {
	Thread string `yaml:"thread"`
	// Scope is once (the default) or thread.
	Scope string `yaml:"scope"`
}

type WaitAction struct {
	Thread string `yaml:"thread"`
	Status string `yaml:"status"`
}

// Expect is checked on Projects once the steps are done.
type Expect struct {
	Threads              *int                    `yaml:"threads"`
	Thread               map[string]ThreadExpect `yaml:"thread"`
	ConversationContains []string                `yaml:"conversation_contains"`
	// MemoryContains are texts some project memory file must contain.
	MemoryContains []string `yaml:"memory_contains"`
}

type ThreadExpect struct {
	Status       string `yaml:"status"`
	CommitsAhead *int   `yaml:"commits_ahead"`
	BranchPrefix string `yaml:"branch_prefix"`
}

// Decode reads a journey, rejecting unknown fields.
func Decode(data []byte) (*Journey, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var j Journey
	if err := dec.Decode(&j); err != nil {
		return nil, fmt.Errorf("decode journey: %w", err)
	}
	if err := j.validate(); err != nil {
		return nil, fmt.Errorf("journey %s: %w", j.Name, err)
	}
	return &j, nil
}

func (j *Journey) validate() error {
	if j.Name == "" {
		return fmt.Errorf("name is required")
	}
	if j.Project.Name == "" {
		return fmt.Errorf("project.name is required")
	}
	if kind, _ := j.Model["kind"].(string); kind != "script" {
		return fmt.Errorf("model must be a vmodel definition of kind script")
	}
	if id, _ := j.Model["id"].(string); id == "" {
		return fmt.Errorf("model.id is required")
	}
	if len(j.Steps) == 0 {
		return fmt.Errorf("steps are required")
	}
	if _, err := j.timeout(); err != nil {
		return err
	}
	for i, a := range j.Steps {
		if err := a.validate(); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}
	return nil
}

func (j *Journey) timeout() (time.Duration, error) {
	if j.Timeout == "" {
		return 60 * time.Second, nil
	}
	d, err := time.ParseDuration(j.Timeout)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("timeout %q must be a positive duration", j.Timeout)
	}
	return d, nil
}

// ModelID is the id of the journey's script definition.
func (j *Journey) ModelID() string { id, _ := j.Model["id"].(string); return id }

func (a Action) validate() error {
	n := 0
	for _, set := range []bool{a.Say != "", a.Message != nil, a.Approve != nil, a.Deny != nil, a.Wait != nil, a.Served != "", a.Reached != "", a.Signal != "", a.StartSuggestions != nil, a.Choose != nil, a.Settle} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("set exactly one of say, message, approve, deny, wait, served, reached, signal, start_suggestions, choose, settle")
	}
	switch {
	case a.Message != nil && (a.Message.Thread == "" || a.Message.Content == ""):
		return fmt.Errorf("message needs thread and content")
	case a.Message != nil && a.Message.Mode != "" && a.Message.Mode != "insert" && a.Message.Mode != "next_turn":
		return fmt.Errorf("message.mode is insert or next_turn")
	case a.Approve != nil && a.Approve.Thread == "", a.Deny != nil && a.Deny.Thread == "":
		return fmt.Errorf("approve/deny needs thread")
	case a.Wait != nil && (a.Wait.Thread == "" || a.Wait.Status == ""):
		return fmt.Errorf("wait needs thread and status")
	case a.Served != "" && !strings.Contains(a.Served, "."), a.Reached != "" && !strings.Contains(a.Reached, "."):
		return fmt.Errorf("served and reached are track.step")
	}
	return nil
}

// String describes the action for the run's output.
func (a Action) String() string {
	switch {
	case a.Say != "":
		return "say " + quote(a.Say)
	case a.Message != nil:
		return "message " + a.Message.Thread + " " + quote(a.Message.Content)
	case a.Approve != nil:
		return "approve " + a.Approve.Thread
	case a.Deny != nil:
		return "deny " + a.Deny.Thread
	case a.Wait != nil:
		return "wait " + a.Wait.Thread + " " + a.Wait.Status
	case a.Served != "":
		return "served " + a.Served
	case a.Reached != "":
		return "reached " + a.Reached
	case a.StartSuggestions != nil:
		return fmt.Sprintf("start suggestions %v", a.StartSuggestions.Indexes)
	case a.Choose != nil:
		return fmt.Sprintf("choose option %d", a.Choose.Option)
	case a.Settle:
		return "settle"
	}
	return "signal " + a.Signal
}

func quote(s string) string {
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return fmt.Sprintf("%q", s)
}

//go:embed journeys/*.yaml
var builtin embed.FS

// Builtin lists the journeys shipped with the binary.
func Builtin() []string {
	entries, _ := fs.ReadDir(builtin, "journeys")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// Load reads a journey file, or a builtin journey by name.
func Load(nameOrPath string) (*Journey, error) {
	data, err := os.ReadFile(nameOrPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		data, err = builtin.ReadFile(path.Join("journeys", nameOrPath+".yaml"))
		if err != nil {
			return nil, fmt.Errorf("no journey file %s and no builtin journey of that name (builtin: %s)", nameOrPath, strings.Join(Builtin(), ", "))
		}
	}
	return Decode(data)
}
