package decision

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"sort"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// Chooser picks the index of the answer among n options. body is the request
// with "model" removed, so a chooser can be input-stable but model-independent.
type Chooser func(body []byte, n int) int

// First always answers with the first option: the predictable choice for
// routing demos and dry-runs.
func First(_ []byte, _ int) int { return 0 }

// Stable answers by hashing the request: the same input always yields the same
// option, different inputs spread across the options. Reproducible without
// looking like a constant.
func Stable(body []byte, n int) int {
	sum := sha256.Sum256(body)
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(n))
}

// MockModelConfig configures a MockModel.
type MockModelConfig struct {
	ID          string
	Name        string
	Description string
	Choose      Chooser
	// Confidence is the probability assigned to the chosen option; the rest is
	// split evenly. Zero means 1 (certain).
	Confidence float64
	Delay      time.Duration
}

// MockModel is a deterministic, in-memory decision model.
type MockModel struct {
	vmodel.BaseMockModel
	choose     Chooser
	confidence float64
}

// NewMockModel builds a MockModel from cfg.
func NewMockModel(cfg *MockModelConfig) *MockModel {
	choose, conf := cfg.Choose, cfg.Confidence
	if choose == nil {
		choose = First
	}
	if conf <= 0 || conf > 1 {
		conf = 1
	}
	return &MockModel{
		BaseMockModel: vmodel.BaseMockModel{
			ID: cfg.ID, Name: cfg.Name, Description: cfg.Description,
			Type: vmodel.VirtualModelTypeDecision, Delay: cfg.Delay,
		},
		choose:     choose,
		confidence: conf,
	}
}

// HandleDecision implements VirtualModel.
func (m *MockModel) HandleDecision(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, badRequest("invalid request body: not valid JSON")
	}
	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}
	// The model name must not influence the answer, only the input does.
	input, _ := sjson.DeleteBytes(body, "model")

	resp := map[string]any{"object": "decision", "model": m.ID}
	answered := 0

	if opts := gjson.GetBytes(body, "options"); opts.Exists() {
		labels, err := optionLabels(opts)
		if err != nil {
			return nil, err
		}
		ans, probs := m.pick(input, labels)
		resp["answer"], resp["probabilities"] = ans, probs
		answered++
	}

	if qs := gjson.GetBytes(body, "questions"); qs.Exists() {
		if !qs.IsObject() || len(qs.Map()) == 0 {
			return nil, badRequest("questions must be a non-empty object")
		}
		answers, probs := map[string]any{}, map[string]any{}
		qmap := qs.Map()
		ids := make([]string, 0, len(qmap))
		for id := range qmap {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			labels, err := optionLabels(qmap[id].Get("options"))
			if err != nil {
				return nil, badRequest("question %q: %s", id, err.Error())
			}
			// Mix the id in so identical questions under different ids may differ.
			ans, p := m.pick(append(append([]byte{}, input...), id...), labels)
			answers[id], probs[id] = ans, p
			answered++
		}
		resp["answers"] = answers
		if _, single := resp["probabilities"]; !single {
			resp["probabilities"] = probs
		} else {
			resp["question_probabilities"] = probs
		}
	}

	if answered == 0 {
		return nil, badRequest("request needs options or questions")
	}

	in := len(body)/4 + 1
	resp["usage"] = map[string]any{"prompt_tokens": in, "completion_tokens": answered, "total_tokens": in + answered}
	return json.Marshal(resp)
}

// pick chooses one label and returns it with the probability table (chosen gets
// the confidence, the remainder is split evenly).
func (m *MockModel) pick(input []byte, labels []string) (string, map[string]float64) {
	idx := m.choose(input, len(labels))
	probs := make(map[string]float64, len(labels))
	rest := 0.0
	if len(labels) > 1 {
		rest = (1 - m.confidence) / float64(len(labels)-1)
	}
	for i, l := range labels {
		if i == idx {
			probs[l] = m.confidence
		} else {
			probs[l] = rest
		}
	}
	return labels[idx], probs
}

// optionLabels reads an options array: strings, or objects naming the option
// by id / label / value / name. Duplicates and empties are rejected so every
// option is addressable.
func optionLabels(opts gjson.Result) ([]string, error) {
	if !opts.IsArray() || len(opts.Array()) == 0 {
		return nil, badRequest("options must be a non-empty array")
	}
	seen := map[string]bool{}
	var out []string
	for i, o := range opts.Array() {
		label := o.String()
		if o.IsObject() {
			label = ""
			for _, k := range []string{"id", "label", "value", "name"} {
				if v := o.Get(k); v.Type == gjson.String && v.String() != "" {
					label = v.String()
					break
				}
			}
		}
		if label == "" {
			return nil, badRequest("option %d has no usable label", i)
		}
		if seen[label] {
			return nil, badRequest("duplicate option %q", label)
		}
		seen[label] = true
		out = append(out, label)
	}
	return out, nil
}
