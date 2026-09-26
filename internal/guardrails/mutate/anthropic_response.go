package mutate

import (
	"encoding/json"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/sjson"

	guardrailscore "github.com/tingly-dev/tingly-box/internal/guardrails/core"
	guardrailsevaluate "github.com/tingly-dev/tingly-box/internal/guardrails/evaluate"
)

// MutateAnthropicV1BetaResponse applies Guardrails evaluation output to a fully
// assembled Anthropic beta response.
func MutateAnthropicV1BetaResponse(resp *anthropic.BetaMessage, evaluation guardrailsevaluate.Evaluation) (bool, string) {
	if resp == nil || evaluation.Result.Verdict != guardrailscore.VerdictBlock {
		return false, ""
	}

	blockMessage := BlockMessageForEvaluation(evaluation)
	resp.Content = []anthropic.BetaContentBlockUnion{{
		Type: "text",
		Text: blockMessage,
	}}
	resp.StopReason = anthropic.BetaStopReasonEndTurn
	syncAnthropicMessageRaw(resp, resp.RawJSON(), blockedMessagePatch(string(resp.Model), blockMessage))
	return true, blockMessage
}

func RestoreAnthropicV1BetaResponseCredentials(state *guardrailscore.CredentialMaskState, resp *anthropic.BetaMessage) bool {
	if resp == nil {
		return false
	}
	if !restoreAnthropicBetaResponseBlocks(resp.Content, state) {
		return false
	}
	blocks := make([]restoredBlock, len(resp.Content))
	for i, block := range resp.Content {
		blocks[i] = restoredBlock{Type: block.Type, Text: block.Text, Input: block.Input}
	}
	syncAnthropicMessageRaw(resp, resp.RawJSON(), restoredBlocksPatch(string(resp.Model), blocks))
	return true
}

func restoreAnthropicBetaResponseBlocks(blocks []anthropic.BetaContentBlockUnion, state *guardrailscore.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrailscore.MayContainAliasToken(block.Text) {
			if text, ok := guardrailscore.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrailscore.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrailscore.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrailscore.RestoreStructuredValue(parsed, state)
		if !ok {
			continue
		}
		payload, err := json.Marshal(restored)
		if err != nil {
			continue
		}
		block.Input = payload
		changed = true
	}
	return changed
}

// Response writers (nonstream.WriteAnthropicMessage) emit the SDK message's
// RawJSON — the upstream body — rather than marshalling the struct, whose
// zero-value fields strict clients reject. A mutation made only on the struct
// would therefore never reach the client. syncAnthropicMessageRaw applies the
// same mutation to the raw JSON and decodes it back into msg so RawJSON and
// the struct agree. Messages without RawJSON are left as-is: their writers
// marshal the struct.
func syncAnthropicMessageRaw[T any](msg *T, raw string, patch func(string) (string, error)) {
	if raw == "" {
		return
	}
	patched, err := patch(raw)
	if err != nil {
		return
	}
	var fresh T
	if err := json.Unmarshal([]byte(patched), &fresh); err != nil {
		return
	}
	*msg = fresh
}

// blockedMessagePatch replaces the whole content with the block message and
// ends the turn. model carries any public-model rewrite already applied to
// the struct, which the re-decode would otherwise revert.
func blockedMessagePatch(model, blockMessage string) func(string) (string, error) {
	return func(raw string) (string, error) {
		content, err := json.Marshal([]map[string]string{{"type": "text", "text": blockMessage}})
		if err != nil {
			return "", err
		}
		raw, err = sjson.SetRaw(raw, "content", string(content))
		if err != nil {
			return "", err
		}
		raw, err = sjson.Set(raw, "stop_reason", string(anthropic.StopReasonEndTurn))
		if err != nil {
			return "", err
		}
		return sjson.Set(raw, "model", model)
	}
}

type restoredBlock struct {
	Type  string
	Text  string
	Input json.RawMessage
}

// restoredBlocksPatch writes credential-restored text and tool input back
// into the matching raw content blocks.
func restoredBlocksPatch(model string, blocks []restoredBlock) func(string) (string, error) {
	return func(raw string) (string, error) {
		var err error
		for i, block := range blocks {
			path := "content." + strconv.Itoa(i)
			if block.Type == "text" {
				if raw, err = sjson.Set(raw, path+".text", block.Text); err != nil {
					return "", err
				}
			}
			if len(block.Input) > 0 {
				if raw, err = sjson.SetRaw(raw, path+".input", string(block.Input)); err != nil {
					return "", err
				}
			}
		}
		return sjson.Set(raw, "model", model)
	}
}
