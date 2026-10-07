package decision

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Jev's documented request: one choice, one score, one noul question.
const jevRequest = `{
  "model": "decision-first",
  "state": "My payouts have failed for three days. Please help me resolve this today.",
  "questions": {
    "department": {"type": "choice", "instructions": "Which team?",
      "criteria": {"billing": "Payments", "technical": "Bugs", "sales": "Pricing"}},
    "frustration": {"type": "score", "instructions": "How frustrated?",
      "criteria": ["Calm", "Frustrated", "Very angry"]},
    "is_urgent": {"type": "noul", "instructions": "Is this urgent?"}
  }
}`

func newReg() *Registry {
	r := NewRegistry()
	RegisterDefaults(r)
	return r
}

func TestFirst_JevShapedAnswers(t *testing.T) {
	out, err := newReg().Get("decision-first").HandleDecision([]byte(jevRequest))
	require.NoError(t, err)

	require.Equal(t, "decision-first", gjson.GetBytes(out, "model").String())

	dep := gjson.GetBytes(out, "answers.department")
	require.Equal(t, "choice", dep.Get("type").String())
	require.Equal(t, "billing", dep.Get("choice").String()) // first criteria key, request order
	require.Equal(t, 1.0, dep.Get("probabilities.billing").Float())
	require.Equal(t, 0.0, dep.Get("probabilities.sales").Float())
	require.Equal(t, 1.0, dep.Get("confidence").Float())

	fr := gjson.GetBytes(out, "answers.frustration")
	require.Equal(t, "score", fr.Get("type").String())
	require.Equal(t, 0.0, fr.Get("score").Float())
	require.Equal(t, "Calm", fr.Get("legend.0").String())
	require.Equal(t, "Very angry", fr.Get("legend.2").String())
	require.Equal(t, 1.0, fr.Get("probabilities.0").Float())

	require.Equal(t, "noul", gjson.GetBytes(out, "answers.is_urgent.type").String())
	require.Equal(t, 1.0, gjson.GetBytes(out, "answers.is_urgent.noul").Float())

	require.Greater(t, gjson.GetBytes(out, "usage.input_tokens").Int(), int64(0))
	require.Greater(t, gjson.GetBytes(out, "usage.output_tokens").Int(), int64(0))
}

func TestStable_DeterministicSpreadAndConsistent(t *testing.T) {
	m := newReg().Get("decision-stable")
	body := func(state string) []byte {
		return []byte(`{"model":"decision-stable","state":"` + state + `","questions":{
			"q":{"type":"choice","criteria":{"a":"","b":"","c":"","d":""}},
			"s":{"type":"score","criteria":["l0","l1","l2","l3"]}}}`)
	}
	a1, err := m.HandleDecision(body("hello"))
	require.NoError(t, err)
	a2, _ := m.HandleDecision(body("hello"))
	require.Equal(t, string(a1), string(a2), "same input, same answer")

	// Whitespace in a structured state must not change the pick.
	p1, _ := m.HandleDecision([]byte(`{"state":{"a":1,"b":[1,2]},"questions":{"q":{"type":"choice","criteria":{"x":"","y":"","z":""}}}}`))
	p2, _ := m.HandleDecision([]byte(`{"state": { "a": 1,  "b": [1, 2] }, "questions":{"q":{"type":"choice","criteria":{"x":"","y":"","z":""}}}}`))
	require.Equal(t, gjson.GetBytes(p1, "answers.q.choice").String(), gjson.GetBytes(p2, "answers.q.choice").String())

	seen := map[string]bool{}
	for _, s := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"} {
		out, _ := m.HandleDecision(body(s))
		seen[gjson.GetBytes(out, "answers.q.choice").String()] = true
	}
	require.Greater(t, len(seen), 1, "different inputs should spread across options")

	// Distribution sums to 1, the pick carries 0.8, and the score is the
	// probability-weighted level.
	q := gjson.GetBytes(a1, "answers.q")
	var sum float64
	q.Get("probabilities").ForEach(func(_, v gjson.Result) bool { sum += v.Float(); return true })
	require.InDelta(t, 1.0, sum, 1e-9)
	require.InDelta(t, 0.8, q.Get("probabilities").Get(q.Get("choice").String()).Float(), 1e-9)
	s := gjson.GetBytes(a1, "answers.s")
	var weighted float64
	s.Get("probabilities").ForEach(func(k, v gjson.Result) bool { weighted += float64(k.Int()) * v.Float(); return true })
	require.InDelta(t, weighted, s.Get("score").Float(), 1e-9)
}

func TestSingleCriterionIsCertain(t *testing.T) {
	out, err := newReg().Get("decision-stable").HandleDecision(
		[]byte(`{"state":"x","questions":{"q":{"type":"choice","criteria":{"only":"d"}}}}`))
	require.NoError(t, err)
	require.Equal(t, 1.0, gjson.GetBytes(out, "answers.q.probabilities.only").Float())
}

func TestRejections(t *testing.T) {
	m := newReg().Get("decision-first")
	for name, raw := range map[string]string{
		"no state":        `{"questions":{"q":{"type":"noul"}}}`,
		"no questions":    `{"state":"x"}`,
		"empty questions": `{"state":"x","questions":{}}`,
		"unknown type":    `{"state":"x","questions":{"q":{"type":"rank"}}}`,
		"missing type":    `{"state":"x","questions":{"q":{}}}`,
		"empty choice":    `{"state":"x","questions":{"q":{"type":"choice","criteria":{}}}}`,
		"one level":       `{"state":"x","questions":{"q":{"type":"score","criteria":["only"]}}}`,
		"eleven levels":   `{"state":"x","questions":{"q":{"type":"score","criteria":["1","2","3","4","5","6","7","8","9","10","11"]}}}`,
	} {
		_, err := m.HandleDecision([]byte(raw))
		var re *RequestError
		require.True(t, errors.As(err, &re), "%s: got %v", name, err)
	}
}
