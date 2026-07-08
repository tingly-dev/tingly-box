package token

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/tiktoken-go/tokenizer"
)

// BenchmarkGetCodecCached measures the memoized codec accessor used by all
// estimators and stream counters.
func BenchmarkGetCodecCached(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := getCodec(tokenizer.O200kBase); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTokenizerGetUncached measures the upstream tokenizer.Get call that
// used to run per request/stream (it recompiles the regexp2 split pattern on
// every invocation).
func BenchmarkTokenizerGetUncached(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := tokenizer.Get(tokenizer.O200kBase); err != nil {
			b.Fatal(err)
		}
	}
}

// benchChunks builds a stream of n content-delta chunks followed by one
// trailing usage chunk, mirroring a provider with include_usage enabled.
func benchChunks(n int) []*openai.ChatCompletionChunk {
	chunks := make([]*openai.ChatCompletionChunk, 0, n+1)
	for i := 0; i < n; i++ {
		chunks = append(chunks, &openai.ChatCompletionChunk{
			Choices: []openai.ChatCompletionChunkChoice{{
				Delta: openai.ChatCompletionChunkChoiceDelta{
					Content: "some streamed delta text with several words in it",
				},
			}},
		})
	}
	usageJSON := `{"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[],` +
		`"usage":{"prompt_tokens":50,"completion_tokens":25,"total_tokens":75}}`
	var usageChunk openai.ChatCompletionChunk
	if err := usageChunk.UnmarshalJSON([]byte(usageJSON)); err != nil {
		panic(err)
	}
	chunks = append(chunks, &usageChunk)
	return chunks
}

// BenchmarkStreamCounterWithUpstreamUsage measures the per-stream token
// accounting cost when the upstream reports usage (the common case): the
// counter should do no BPE tokenization at all.
func BenchmarkStreamCounterWithUpstreamUsage(b *testing.B) {
	chunks := benchChunks(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter, err := NewStreamTokenCounter()
		if err != nil {
			b.Fatal(err)
		}
		for _, c := range chunks {
			_ = counter.ConsumeOpenAIChunk(c)
		}
		if _, out := counter.GetCounts(); out != 25 {
			b.Fatalf("expected upstream output 25, got %d", out)
		}
	}
}

// BenchmarkStreamCounterNoUpstreamUsage measures the fallback path: no usage
// chunk arrives, so the buffered text is tokenized once at the end.
func BenchmarkStreamCounterNoUpstreamUsage(b *testing.B) {
	chunks := benchChunks(200)
	chunks = chunks[:len(chunks)-1] // drop the usage chunk
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter, err := NewStreamTokenCounter()
		if err != nil {
			b.Fatal(err)
		}
		for _, c := range chunks {
			_ = counter.ConsumeOpenAIChunk(c)
		}
		if _, out := counter.GetCounts(); out == 0 {
			b.Fatal("expected estimated output tokens > 0")
		}
	}
}
