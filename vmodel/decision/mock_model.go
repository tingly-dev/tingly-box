package decision

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"strconv"

	"github.com/tidwall/gjson"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// Chooser picks an index in [0, n) for one question. input is the request's
// state and id the question id, so a chooser can be input-stable but
// independent of the model name.
type Chooser func(input []byte, id string, n int) int

// First always picks index 0: the first choice option, the lowest score level,
// and "true" for noul. Predictable for routing demos and dry-runs.
func First(_ []byte, _ string, _ int) int { return 0 }

// Stable hashes state+id: the same input always gets the same pick, different
// inputs spread across the options.
func Stable(input []byte, id string, n int) int {
	sum := sha256.Sum256(append(append(append([]byte{}, input...), 0), id...))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(n))
}

// MockModelConfig configures a MockModel.
type MockModelConfig struct {
	ID          string
	Name        string
	Description string
	Choose      Chooser
	// Confidence is the probability mass on the picked option/level (and the
	// noul probability when it picks "true"); the rest is split evenly. Zero
	// means 1 (certain).
	Confidence float64
}

// MockModel is a deterministic, in-memory decision model.
type MockModel struct {
	vmodel.BaseMockModel
	choose Chooser
	mass   float64
}

// NewMockModel builds a MockModel from cfg.
func NewMockModel(cfg *MockModelConfig) *MockModel {
	choose, mass := cfg.Choose, cfg.Confidence
	if choose == nil {
		choose = First
	}
	if mass <= 0 || mass > 1 {
		mass = 1
	}
	return &MockModel{
		BaseMockModel: vmodel.BaseMockModel{
			ID: cfg.ID, Name: cfg.Name, Description: cfg.Description,
			Type: vmodel.VirtualModelTypeDecision,
		},
		choose: choose,
		mass:   mass,
	}
}

// HandleDecision implements VirtualModel.
func (m *MockModel) HandleDecision(body []byte) ([]byte, error) {
	root := gjson.ParseBytes(body)
	state := root.Get("state")
	if !state.Exists() {
		return nil, badRequest("state is required")
	}
	qs := root.Get("questions")
	if !qs.IsObject() || len(qs.Map()) == 0 {
		return nil, badRequest("questions must be a non-empty object")
	}

	// Compacted so the pick does not depend on the client's whitespace.
	var input bytes.Buffer
	if err := json.Compact(&input, []byte(state.Raw)); err != nil {
		return nil, badRequest("invalid state: %v", err)
	}

	answers := make(map[string]any, len(qs.Map()))
	var verr error
	qs.ForEach(func(k, q gjson.Result) bool {
		var a map[string]any
		a, verr = m.answer(input.Bytes(), k.String(), q)
		if verr != nil {
			verr = badRequest("question %q: %s", k.String(), verr.Error())
			return false
		}
		answers[k.String()] = a
		return true
	})
	if verr != nil {
		return nil, verr
	}

	return json.Marshal(map[string]any{
		"model":   m.ID,
		"answers": answers,
		"usage": map[string]any{
			"input_tokens":  len(body)/4 + 1,
			"output_tokens": len(answers) * 8,
		},
	})
}

func (m *MockModel) answer(input []byte, id string, q gjson.Result) (map[string]any, error) {
	switch typ := q.Get("type").String(); typ {
	case "choice":
		var names []string
		if c := q.Get("criteria"); c.IsObject() {
			c.ForEach(func(k, _ gjson.Result) bool { names = append(names, k.String()); return true })
		}
		if len(names) == 0 {
			return nil, badRequest("choice needs a non-empty criteria object")
		}
		idx := m.choose(input, id, len(names))
		probs := m.distribution(len(names), idx)
		byName := make(map[string]float64, len(names))
		for i, n := range names {
			byName[n] = probs[i]
		}
		return map[string]any{"type": "choice", "choice": names[idx], "probabilities": byName, "confidence": confidence(probs)}, nil

	case "score":
		levels := q.Get("criteria").Array()
		if len(levels) < 2 || len(levels) > 10 {
			return nil, badRequest("score needs a criteria array of 2 to 10 levels")
		}
		idx := m.choose(input, id, len(levels))
		probs := m.distribution(len(levels), idx)
		legend, byLevel, score := map[string]string{}, map[string]float64{}, 0.0
		for i, l := range levels {
			k := strconv.Itoa(i)
			legend[k], byLevel[k] = l.String(), probs[i]
			score += float64(i) * probs[i]
		}
		return map[string]any{"type": "score", "score": score, "legend": legend, "probabilities": byLevel, "confidence": confidence(probs)}, nil

	case "noul":
		p := m.mass
		if m.choose(input, id, 2) != 0 {
			p = 1 - m.mass
		}
		return map[string]any{"type": "noul", "noul": p}, nil

	default:
		return nil, badRequest("unsupported type %q (want choice, score or noul)", typ)
	}
}

// distribution puts the configured mass on idx and splits the rest evenly; a
// single candidate always gets everything.
func (m *MockModel) distribution(n, idx int) []float64 {
	probs := make([]float64, n)
	if n == 1 {
		probs[0] = 1
		return probs
	}
	rest := (1 - m.mass) / float64(n-1)
	for i := range probs {
		probs[i] = rest
	}
	probs[idx] = m.mass
	return probs
}

// confidence is 1 minus the normalized entropy of the distribution: 1 when one
// outcome is certain, 0 when all are equally likely.
func confidence(probs []float64) float64 {
	if len(probs) < 2 {
		return 1
	}
	h := 0.0
	for _, p := range probs {
		if p > 0 {
			h -= p * math.Log(p)
		}
	}
	c := 1 - h/math.Log(float64(len(probs)))
	return math.Round(c*1e4) / 1e4
}
