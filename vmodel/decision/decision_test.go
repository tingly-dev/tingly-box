package decision

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newReg() *Registry {
	r := NewRegistry()
	RegisterDefaults(r)
	return r
}

func TestFirst_SingleChoice(t *testing.T) {
	out, err := newReg().Get("decision-first").HandleDecision(
		[]byte(`{"model":"decision-first","options":["billing","bug",{"id":"sales"}],"context":"x"}`))
	require.NoError(t, err)
	require.Equal(t, "billing", gjson.GetBytes(out, "answer").String())
	require.Equal(t, 1.0, gjson.GetBytes(out, "probabilities.billing").Float())
	require.Equal(t, 0.0, gjson.GetBytes(out, "probabilities.sales").Float())
	require.Equal(t, "decision-first", gjson.GetBytes(out, "model").String())
	require.True(t, gjson.GetBytes(out, "usage.prompt_tokens").Int() > 0)
}

func TestStable_DeterministicAndInputDependent(t *testing.T) {
	m := newReg().Get("decision-stable")
	body := func(ctx string) []byte {
		return []byte(`{"model":"decision-stable","context":"` + ctx + `","options":["a","b","c","d"]}`)
	}
	a1, _ := m.HandleDecision(body("hello"))
	a2, _ := m.HandleDecision(body("hello"))
	require.Equal(t, string(a1), string(a2))

	seen := map[string]bool{}
	for _, c := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"} {
		out, err := m.HandleDecision(body(c))
		require.NoError(t, err)
		seen[gjson.GetBytes(out, "answer").String()] = true
	}
	require.Greater(t, len(seen), 1, "different inputs should spread across options")

	// Probabilities form a distribution with the chosen option at 0.8.
	var sum float64
	gjson.GetBytes(a1, "probabilities").ForEach(func(_, v gjson.Result) bool { sum += v.Float(); return true })
	require.InDelta(t, 1.0, sum, 1e-9)
	require.InDelta(t, 0.8, gjson.GetBytes(a1, "probabilities").Get(gjson.GetBytes(a1, "answer").String()).Float(), 1e-9)
}

func TestQuestionsShape(t *testing.T) {
	out, err := newReg().Get("decision-first").HandleDecision([]byte(
		`{"model":"m","state":{"k":1},"questions":{"urgent":{"type":"choice","options":["yes","no"]},"a.b":{"options":["x"]}}}`))
	require.NoError(t, err)
	require.Equal(t, "yes", gjson.GetBytes(out, "answers.urgent").String())
	require.Equal(t, "x", gjson.GetBytes(out, `answers.a\.b`).String())
	require.Equal(t, 1.0, gjson.GetBytes(out, "probabilities.urgent.yes").Float())
}

func TestRejections(t *testing.T) {
	m := newReg().Get("decision-first")
	for name, raw := range map[string]string{
		"not json":       `[{`,
		"nothing":        `{"model":"m"}`,
		"empty options":  `{"options":[]}`,
		"dup options":    `{"options":["a","a"]}`,
		"unlabelled":     `{"options":[{"x":1}]}`,
		"empty question": `{"questions":{}}`,
		"no q options":   `{"questions":{"q":{"type":"score"}}}`,
	} {
		_, err := m.HandleDecision([]byte(raw))
		var re *RequestError
		require.True(t, errors.As(err, &re), "%s: got %v", name, err)
	}
}
