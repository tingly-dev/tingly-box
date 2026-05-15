package smartrouting

import (
	"context"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// OpProcessor is a side-effect handler bound to a (Position, Operation) tuple.
// When the routing stage matches a rule whose ops include a processor-bearing
// op, every collected processor's Process is invoked and the smart-routing
// stage returns (nil, false) — letting the pipeline continue to the
// LoadBalancer with whatever mutations the processor made to ctx.Request
// (implicit bypass).
type OpProcessor interface {
	Process(pctx *ProcessorContext) error
}

// ProcessorContext carries everything a processor needs. Request is the typed
// request struct (e.g. *anthropic.BetaMessageNewParams) and may be mutated
// in place. Services is the matched rule's Services slice — the processor's
// upstream candidate pool, NOT the downstream selection set.
type ProcessorContext struct {
	Ctx       context.Context
	Request   any
	ReqCtx    *RequestContext
	RuleIndex int
	OpUUID    string
	Services  []*loadbalance.Service
}

var (
	processorRegistryMu sync.RWMutex
	processorRegistry   = make(map[string]OpProcessor)
)

func processorKey(pos SmartOpPosition, op SmartOpOperation) string {
	return string(pos) + "|" + string(op)
}

// RegisterProcessor installs a processor for (pos, op). Subsequent calls
// silently replace the prior registration — keeps server boot idempotent
// across config reloads.
func RegisterProcessor(pos SmartOpPosition, op SmartOpOperation, p OpProcessor) {
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	processorRegistry[processorKey(pos, op)] = p
}

// LookupProcessor retrieves a processor for (pos, op) if registered.
func LookupProcessor(pos SmartOpPosition, op SmartOpOperation) (OpProcessor, bool) {
	processorRegistryMu.RLock()
	defer processorRegistryMu.RUnlock()
	p, ok := processorRegistry[processorKey(pos, op)]
	return p, ok
}

// UnregisterProcessor removes a processor; primarily used by tests via
// t.Cleanup but also valid for runtime deregistration.
func UnregisterProcessor(pos SmartOpPosition, op SmartOpOperation) {
	processorRegistryMu.Lock()
	defer processorRegistryMu.Unlock()
	delete(processorRegistry, processorKey(pos, op))
}
