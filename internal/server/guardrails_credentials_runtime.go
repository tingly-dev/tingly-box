package server

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const guardrailsCredentialMaskPreviewLimit = 160

// Protected credentials are request-side content rewrites, not terminal blocks.
// Apply them before the normal block/review path so the model only sees alias
// tokens instead of the real secret values.
func (s *Server) applyGuardrailsCredentialMasksV1(c *gin.Context, req *anthropic.MessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}
	session := s.guardrailsSessionFromContext(c, actualModel, provider)
	s.applyGuardrailsCredentialMasksToV1Request(c, session, req)
}

func (s *Server) applyGuardrailsCredentialMasksV1WithSession(c *gin.Context, req *anthropic.MessageNewParams, session guardrailsSession) {
	if req == nil {
		return
	}
	s.applyGuardrailsCredentialMasksToV1Request(c, session, req)
}

func (s *Server) applyGuardrailsCredentialMasksV1Beta(c *gin.Context, req *anthropic.BetaMessageNewParams, actualModel string, provider *typ.Provider) {
	if req == nil {
		return
	}
	session := s.guardrailsSessionFromContext(c, actualModel, provider)
	s.applyGuardrailsCredentialMasksToV1BetaRequest(c, session, req)
}

func (s *Server) applyGuardrailsCredentialMasksV1BetaWithSession(c *gin.Context, req *anthropic.BetaMessageNewParams, session guardrailsSession) {
	if req == nil {
		return
	}
	s.applyGuardrailsCredentialMasksToV1BetaRequest(c, session, req)
}

// getGuardrailsCredentialMaskState returns the request-scoped alias map shared by
// request masking, non-stream response restoration, and stream restoration.
func (s *Server) getGuardrailsCredentialMaskState(c *gin.Context) *guardrails.CredentialMaskState {
	if existing, ok := c.Get(guardrails.CredentialMaskStateContextKey); ok {
		if state, ok := existing.(*guardrails.CredentialMaskState); ok {
			return state
		}
	}
	state := guardrails.NewCredentialMaskState()
	c.Set(guardrails.CredentialMaskStateContextKey, state)
	return state
}

// loadActiveMaskCredentials returns the enabled protected credentials for the
// current scenario from the prebuilt cache.
func (s *Server) loadActiveMaskCredentials(session guardrailsSession) ([]guardrails.ProtectedCredential, error) {
	if !s.guardrailsEnabledForSession(session) {
		return nil, nil
	}
	return s.getCachedGuardrailsMaskCredentials(session.Scenario), nil
}

// applyGuardrailsCredentialMasksToV1Request rewrites Anthropic v1 request content
// in place so upstream models only receive alias tokens.
func (s *Server) applyGuardrailsCredentialMasksToV1Request(c *gin.Context, session guardrailsSession, req *anthropic.MessageNewParams) {
	//roundPreview := currentAnthropicV1RoundPreview(req.Messages)
	//logGuardrailsCredentialMaskRequest("v1", session, len(req.System), len(req.Messages), roundPreview)
	credentials, err := s.loadActiveMaskCredentials(session)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails credential mask: failed to load credentials")
		return
	}
	if len(credentials) == 0 {
		return
	}
	state := s.getGuardrailsCredentialMaskState(c)
	changed := false
	latestBlockChanged := false
	for i := range req.System {
		if next, ok := guardrails.AliasText(req.System[i].Text, credentials, state); ok {
			req.System[i].Text = next
			changed = true
		}
	}
	for i := range req.Messages {
		messageChanged, tailChanged := aliasAnthropicMessageBlocks(req.Messages[i].Content, credentials, state)
		if messageChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && tailChanged {
			latestBlockChanged = true
		}
	}
	if changed && latestBlockChanged {
		input := s.buildGuardrailsBaseInput(session, guardrails.DirectionRequest, serverguardrails.MessagesFromAnthropicV1(req.System, req.Messages))
		s.recordGuardrailsMaskHistory(c, session, input, "request_mask")
		logrus.Debugf("Guardrails credential mask applied (v1) refs=%d", len(state.UsedRefs))
	}
}

// applyGuardrailsCredentialMasksToV1BetaRequest applies the same request-side
// masking to Anthropic beta payloads.
func (s *Server) applyGuardrailsCredentialMasksToV1BetaRequest(c *gin.Context, session guardrailsSession, req *anthropic.BetaMessageNewParams) {
	//roundPreview := currentAnthropicBetaRoundPreview(req.Messages)
	//logGuardrailsCredentialMaskRequest("v1beta", session, len(req.System), len(req.Messages), roundPreview)
	credentials, err := s.loadActiveMaskCredentials(session)
	if err != nil {
		logrus.WithError(err).Debug("Guardrails credential mask: failed to load credentials")
		return
	}
	if len(credentials) == 0 {
		return
	}
	state := s.getGuardrailsCredentialMaskState(c)
	changed := false
	latestBlockChanged := false
	for i := range req.System {
		if next, ok := guardrails.AliasText(req.System[i].Text, credentials, state); ok {
			req.System[i].Text = next
			changed = true
		}
	}
	for i := range req.Messages {
		messageChanged, tailChanged := aliasAnthropicBetaMessageBlocks(req.Messages[i].Content, credentials, state)
		if messageChanged {
			changed = true
		}
		if i == len(req.Messages)-1 && tailChanged {
			latestBlockChanged = true
		}
	}
	// Claude Code resends the full history on follow-up requests. Only record mask
	// history when the newest message in the request was actually rewritten; this
	// keeps history focused on the user-visible turn instead of repeated context
	// replay.
	if changed && latestBlockChanged {
		input := s.buildGuardrailsBaseInput(session, guardrails.DirectionRequest, serverguardrails.MessagesFromAnthropicV1Beta(req.System, req.Messages))
		s.recordGuardrailsMaskHistory(c, session, input, "request_mask")
		logrus.Debugf("Guardrails credential mask applied (v1beta) refs=%d", len(state.UsedRefs))
	}
}

// aliasAnthropicMessageBlocks rewrites all supported block types in one message
// and reports whether the trailing block changed for history suppression.
func aliasAnthropicMessageBlocks(blocks []anthropic.ContentBlockParamUnion, credentials []guardrails.ProtectedCredential, state *guardrails.CredentialMaskState) (bool, bool) {
	changed := false
	tailChanged := false
	for i := range blocks {
		block := &blocks[i]
		blockChanged := false
		if block.OfText != nil {
			if next, ok := guardrails.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
				blockChanged = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrails.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
						blockChanged = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrails.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
					blockChanged = true
				}
			}
		}
		if i == len(blocks)-1 && blockChanged {
			tailChanged = true
		}
	}
	return changed, tailChanged
}

// aliasAnthropicBetaMessageBlocks is the beta equivalent of
// aliasAnthropicMessageBlocks.
func aliasAnthropicBetaMessageBlocks(blocks []anthropic.BetaContentBlockParamUnion, credentials []guardrails.ProtectedCredential, state *guardrails.CredentialMaskState) (bool, bool) {
	changed := false
	tailChanged := false
	for i := range blocks {
		block := &blocks[i]
		blockChanged := false
		if block.OfText != nil {
			if next, ok := guardrails.AliasText(block.OfText.Text, credentials, state); ok {
				block.OfText.Text = next
				changed = true
				blockChanged = true
			}
		}
		if block.OfToolResult != nil {
			for j := range block.OfToolResult.Content {
				content := &block.OfToolResult.Content[j]
				if content.OfText != nil {
					if next, ok := guardrails.AliasText(content.OfText.Text, credentials, state); ok {
						content.OfText.Text = next
						changed = true
						blockChanged = true
					}
				}
			}
		}
		if block.OfToolUse != nil {
			if next, ok := guardrails.AliasStructuredValue(block.OfToolUse.Input, credentials, state); ok {
				if args, ok := next.(map[string]interface{}); ok {
					block.OfToolUse.Input = args
					changed = true
					blockChanged = true
				}
			}
		}
		if i == len(blocks)-1 && blockChanged {
			tailChanged = true
		}
	}
	return changed, tailChanged
}

func logGuardrailsCredentialMaskRequest(api string, session guardrailsSession, systemCount, messageCount int, latestUser string) {
	logrus.Debugf(
		"Guardrails credential mask request api=%s scenario=%s messages=%d system=%d latest_user=%q",
		api,
		session.Scenario,
		messageCount,
		systemCount,
		truncateGuardrailsCredentialMaskPreview(latestUser),
	)
}

func currentAnthropicV1RoundPreview(messages []anthropic.MessageParam) string {
	rounds := protocol.NewGrouper().GroupV1(messages)
	if len(rounds) == 0 {
		return ""
	}
	return anthropicV1MessageText(rounds[len(rounds)-1].Messages[0])
}

func currentAnthropicBetaRoundPreview(messages []anthropic.BetaMessageParam) string {
	rounds := protocol.NewGrouper().GroupBeta(messages)
	if len(rounds) == 0 {
		return ""
	}
	return anthropicBetaMessageText(rounds[len(rounds)-1].Messages[0])
}

func anthropicV1MessageText(message anthropic.MessageParam) string {
	for _, block := range message.Content {
		if block.OfText != nil && strings.TrimSpace(block.OfText.Text) != "" {
			return block.OfText.Text
		}
	}
	return ""
}

func anthropicBetaMessageText(message anthropic.BetaMessageParam) string {
	for _, block := range message.Content {
		if block.OfText != nil && strings.TrimSpace(block.OfText.Text) != "" {
			return block.OfText.Text
		}
	}
	return ""
}

func truncateGuardrailsCredentialMaskPreview(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= guardrailsCredentialMaskPreviewLimit {
		return text
	}
	return text[:guardrailsCredentialMaskPreviewLimit] + "..."
}

// restoreGuardrailsCredentialAliasesV1Response restores locally generated alias
// tokens in Anthropic non-stream responses before they are returned to the
// client. This mirrors the stream-side delta rewrite, but works on the fully
// assembled response object.
func (s *Server) restoreGuardrailsCredentialAliasesV1Response(c *gin.Context, resp *anthropic.Message) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicResponseBlocks(resp.Content, s.getGuardrailsCredentialMaskState(c))
}

// restoreGuardrailsCredentialAliasesV1BetaResponse applies the same response-side
// alias restoration to Anthropic beta non-stream responses.
func (s *Server) restoreGuardrailsCredentialAliasesV1BetaResponse(c *gin.Context, resp *anthropic.BetaMessage) bool {
	if resp == nil {
		return false
	}
	return restoreAnthropicBetaResponseBlocks(resp.Content, s.getGuardrailsCredentialMaskState(c))
}

// restoreAnthropicResponseBlocks restores alias tokens inside non-stream
// Anthropic v1 response blocks.
func restoreAnthropicResponseBlocks(blocks []anthropic.ContentBlockUnion, state *guardrails.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrails.MayContainAliasToken(block.Text) {
			if text, ok := guardrails.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrails.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrails.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrails.RestoreStructuredValue(parsed, state)
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

// restoreAnthropicBetaResponseBlocks restores alias tokens inside non-stream
// Anthropic beta response blocks.
func restoreAnthropicBetaResponseBlocks(blocks []anthropic.BetaContentBlockUnion, state *guardrails.CredentialMaskState) bool {
	if state == nil || len(state.AliasToReal) == 0 {
		return false
	}
	changed := false
	for i := range blocks {
		block := &blocks[i]
		if guardrails.MayContainAliasToken(block.Text) {
			if text, ok := guardrails.RestoreText(block.Text, state); ok {
				block.Text = text
				changed = true
			}
		}
		if len(block.Input) == 0 || !guardrails.MayContainAliasToken(string(block.Input)) {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal(block.Input, &parsed); err != nil {
			if restored, ok := guardrails.RestoreText(string(block.Input), state); ok {
				block.Input = json.RawMessage(restored)
				changed = true
			}
			continue
		}
		restored, ok := guardrails.RestoreStructuredValue(parsed, state)
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
