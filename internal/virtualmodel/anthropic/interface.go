// Package anthropic provides Anthropic-protocol virtual models.
//
// All concrete virtual models in this package implement VirtualModel —
// a base virtualmodel.VirtualModel extended with HandleAnthropic /
// HandleAnthropicStream. Models are stored in a Registry that is
// scoped to this protocol; lookups never cross over to OpenAI.
package anthropic

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// VirtualModel handles Anthropic Beta Messages requests. It embeds the
// provider-neutral virtualmodel.VirtualModel and adds the Handle methods
// for the Anthropic protocol.
type VirtualModel interface {
	virtualmodel.VirtualModel
	HandleAnthropic(req *protocol.AnthropicBetaMessagesRequest) (VModelResponse, error)
	HandleAnthropicStream(req *protocol.AnthropicBetaMessagesRequest, emit func(any)) error
}
