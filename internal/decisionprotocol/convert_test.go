package decisionprotocol

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

func TestConversionOpenAIToSystemOne(t *testing.T) {
	req := []byte(`{"model":"m","input":"hello","questions":[{"type":"predicate","name":"urgent","instructions":"urgent?"},{"type":"choice","name":"dept","instructions":"where?","choices":[{"value":"z","description":"last"},{"value":"a","description":"first"}]},{"type":"score","name":"rating","instructions":"rate","levels":[{"label":"low"},{"label":"high"}]}]}`)
	out, ctx, err := ConvertRequest(req, OpenAI, SystemOne)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"m","state":"hello","questions":{"urgent":{"type":"noul","instructions":"urgent?"},"dept":{"type":"choice","instructions":"where?","criteria":{"z":"last","a":"first"}},"rating":{"type":"score","instructions":"rate","criteria":["low","high"]}}}`, string(out))
	require.Less(t, stringsIndex(string(out), `"z":"last"`), stringsIndex(string(out), `"a":"first"`))
	resp := []byte(`{"model":"m","answers":{"rating":{"type":"score","score":0.8,"confidence":0.4,"legend":{"0":"low","1":"high"},"probabilities":{"0":0.2,"1":0.8}},"dept":{"type":"choice","choice":"z","confidence":0.7,"probabilities":{"z":0.7,"a":0.3}},"urgent":{"type":"noul","noul":0.9}},"usage":{"inputTokens":10,"outputTokens":2}}`)
	got, err := ctx.ConvertResponse(resp)
	require.NoError(t, err)
	require.Equal(t, "urgent", gjson.GetBytes(got, "answers.0.name").String())
	require.Equal(t, 0.9, gjson.GetBytes(got, "answers.0.probability").Float())
	require.Equal(t, "z", gjson.GetBytes(got, "answers.1.probabilities.0.value").String())
	require.Equal(t, "high", gjson.GetBytes(got, "answers.2.probabilities.1.label").String())
	require.Equal(t, int64(12), gjson.GetBytes(got, "usage.total_tokens").Int())
}
func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func TestConversionSystemOneToOpenAI(t *testing.T) {
	req := []byte(`{"model":"m","state":"hello","questions":{"urgent":{"type":"noul","instructions":"urgent?"},"dept":{"type":"choice","instructions":"where?","criteria":{"z":"last","a":"first"}},"rating":{"type":"score","instructions":"rate","criteria":["low","high"]}}}`)
	out, ctx, err := ConvertRequest(req, SystemOne, OpenAI)
	require.NoError(t, err)
	require.Equal(t, "urgent", gjson.GetBytes(out, "questions.0.name").String())
	resp := []byte(`{"model":"m","answers":[{"type":"predicate","name":"urgent","probability":0.9},{"type":"choice","name":"dept","choice":"z","confidence":0.7,"probabilities":[{"value":"z","probability":0.7},{"value":"a","probability":0.3}]},{"type":"score","name":"rating","score":0.8,"confidence":0.4,"probabilities":[{"value":0,"label":"low","probability":0.2},{"value":1,"label":"high","probability":0.8}]}],"usage":{"input_tokens":10,"output_tokens":2}}`)
	got, err := ctx.ConvertResponse(resp)
	require.NoError(t, err)
	require.Equal(t, 0.9, gjson.GetBytes(got, "answers.urgent.noul").Float())
	require.Equal(t, 0.7, gjson.GetBytes(got, "answers.dept.probabilities.z").Float())
	require.Equal(t, "high", gjson.GetBytes(got, "answers.rating.legend.1").String())
}
func TestConversionRejectsUnsupportedBeforeRequest(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","input":"x","questions":[{"type":"choice","name":"q","instructions":"x","choices":[{"value":true},{"value":false}]}]}`,
		`{"model":"m","input":"x","prompt_cache_key":"k","questions":[{"type":"predicate","name":"q","instructions":"x"}]}`,
		`{"model":"m","input":"x","questions":[{"type":"score","name":"q","instructions":"x","levels":[{"label":"a","description":"desc"},{"label":"b"}]}]}`,
		`{"model":"m","input":"x","questions":[{"type":"predicate","name":"q","instructions":"x"},{"type":"predicate","name":"q","instructions":"x"}]}`,
	} {
		_, _, err := ConvertRequest([]byte(body), OpenAI, SystemOne)
		require.Error(t, err, body)
	}
}
func TestConversionRejectsIncompleteResponse(t *testing.T) {
	_, ctx, err := ConvertRequest([]byte(`{"model":"m","input":"x","questions":[{"type":"predicate","name":"q","instructions":"x"}]}`), OpenAI, SystemOne)
	require.NoError(t, err)
	for _, resp := range []string{`{"model":"m","answers":{}}`, `{"model":"m","answers":{"q":{"type":"noul"}}}`, `{"model":"m","answers":{"q":{"type":"choice"}}}`, `{"model":"m","answers":{"q":{"type":"noul","noul":0.8,"extra":1}}}`} {
		_, err := ctx.ConvertResponse([]byte(resp))
		require.Error(t, err)
	}
}
func TestResolveTarget(t *testing.T) {
	for base, want := range map[string]Dialect{"https://api.openai.com/v1": OpenAI, "https://ai-gateway.vercel.sh/v1": OpenAI, "https://ai-gateway.vercel.sh/typesafe": Unknown, "https://api.typesafe.ai/v1": SystemOne, "https://openrouter.ai/api/v1": SystemOne, "https://api.typesafe.ai.example/v1": Unknown} {
		require.Equal(t, want, ResolveTarget(base).Dialect, base)
	}
}
