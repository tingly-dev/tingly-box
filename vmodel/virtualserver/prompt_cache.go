package virtualserver

import (
	"bytes"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/vmodel"
	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/promptcache"
)

// Prompt-cache simulation (vmodel.PromptCacheModel).
//
// The simulation needs the request exactly as the upstream would see it, so it
// runs here on the raw body rather than in the model, which only ever gets the
// decoded struct (and, on the Responses endpoint, only the last user text).
// The model is then swapped for a per-request MockModel whose reply is the
// simulation report, and the handler renders the simulated usage in place of
// its token estimate: the usage on the wire is the thing under test.

// readRawBody returns the request body and restores it for binding.
func readRawBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw
}

// simulatePromptCache runs one request through pc's simulator. endpoint names
// the cache scope together with the model; ok is false when the body could not
// be split into blocks, in which case the caller serves the model as is.
func simulatePromptCache(pc vmodel.PromptCacheModel, endpoint, model string, raw []byte) (res promptcache.Result, ok bool) {
	var (
		blocks []promptcache.Block
		err    error
		d      = promptcache.Automatic
	)
	switch endpoint {
	case "anthropic":
		blocks, err = promptcache.AnthropicBlocks(raw)
		d = promptcache.AnthropicDiscipline(blocks)
	case "chat":
		blocks, err = promptcache.ChatBlocks(raw)
	case "responses":
		blocks, err = promptcache.ResponsesBlocks(raw)
	}
	if err != nil {
		logrus.WithError(err).Debug("[vmodel] prompt-cache: cannot split request")
		return res, false
	}
	res = pc.PromptCache().Observe(endpoint+"/"+model, d, blocks)
	logrus.WithFields(logrus.Fields{
		"endpoint": endpoint, "model": model,
		"prompt_tokens": res.PromptTokens, "cached_tokens": res.CachedTokens, "write_tokens": res.WriteTokens,
		"cached_blocks": res.CachedBlocks, "blocks": res.Blocks, "first_uncached": res.FirstUncached,
	}).Info("[vmodel] prompt-cache simulation")
	return res, true
}

// anthropicPromptCache resolves a prompt-cache model for one Anthropic request.
// It returns vm unchanged and nil usage for any other model.
func anthropicPromptCache(vm anthropicvm.VirtualModel, model string, raw []byte) (anthropicvm.VirtualModel, *vmodel.MockUsage) {
	pc, isPC := vm.(vmodel.PromptCacheModel)
	if !isPC {
		return vm, nil
	}
	res, ok := simulatePromptCache(pc, "anthropic", model, raw)
	if !ok {
		return vm, nil
	}
	report := res.Report()
	return anthropicvm.NewMockModel(&anthropicvm.MockModelConfig{ID: vm.GetID(), Name: vm.GetName(), Content: report}),
		&vmodel.MockUsage{
			// Anthropic's input_tokens excludes both cache reads and writes.
			PromptTokens:      res.UncachedInputTokens(),
			CompletionTokens:  int64(token.EstimateTokensString(report)),
			CachedInputTokens: res.CachedTokens,
			CacheWriteTokens:  res.WriteTokens,
		}
}

// openaiPromptCache is anthropicPromptCache for the OpenAI registry; endpoint
// is "chat" or "responses".
func openaiPromptCache(vm openaivm.VirtualModel, endpoint, model string, raw []byte) (openaivm.VirtualModel, *vmodel.MockUsage) {
	pc, isPC := vm.(vmodel.PromptCacheModel)
	if !isPC {
		return vm, nil
	}
	res, ok := simulatePromptCache(pc, endpoint, model, raw)
	if !ok {
		return vm, nil
	}
	report := res.Report()
	return openaivm.NewMockModel(&openaivm.MockModelConfig{ID: vm.GetID(), Name: vm.GetName(), Content: report}),
		&vmodel.MockUsage{
			// OpenAI's prompt_tokens is the whole prompt, cached part included.
			PromptTokens:      res.PromptTokens,
			CompletionTokens:  int64(token.EstimateTokensString(report)),
			CachedInputTokens: res.CachedTokens,
		}
}

// anthropicUsageFrom renders a preset usage in Anthropic wire form.
func anthropicUsageFrom(u *vmodel.MockUsage) AnthropicUsage {
	return AnthropicUsage{
		InputTokens:              u.PromptTokens,
		OutputTokens:             u.CompletionTokens,
		CacheReadInputTokens:     u.CachedInputTokens,
		CacheCreationInputTokens: u.CacheWriteTokens,
	}
}

// chatUsageFrom renders a preset usage in OpenAI Chat wire form.
func chatUsageFrom(u *vmodel.MockUsage) Usage {
	usage := Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.PromptTokens + u.CompletionTokens,
	}
	usage.PromptTokensDetails.CachedTokens = u.CachedInputTokens
	usage.PromptTokensDetails.CacheWriteTokens = u.CacheWriteTokens
	usage.CompletionTokensDetails.ReasoningTokens = u.ReasoningTokens
	return usage
}

// responsesUsageFrom renders a preset usage in OpenAI Responses wire form.
func responsesUsageFrom(u *vmodel.MockUsage) map[string]interface{} {
	return map[string]interface{}{
		"input_tokens":          u.PromptTokens,
		"output_tokens":         u.CompletionTokens,
		"total_tokens":          u.PromptTokens + u.CompletionTokens,
		"input_tokens_details":  map[string]interface{}{"cached_tokens": u.CachedInputTokens},
		"output_tokens_details": map[string]interface{}{"reasoning_tokens": u.ReasoningTokens},
	}
}
