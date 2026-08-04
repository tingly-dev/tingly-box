package access

import "context"

// Evaluate applies the single, stable authorization order. It is deliberately
// pure so production, diagnostics, and table tests cannot drift apart.
func Evaluate(req AuthorizationRequest, facts DecisionFacts) AuthorizationDecision {
	deny := func(gate GateName, reason DecisionReason) AuthorizationDecision {
		return AuthorizationDecision{Reason: reason, FailedGate: gate, Facts: facts}
	}

	if !facts.BotEnabled {
		return deny(GateBot, ReasonBotDisabled)
	}
	if !facts.CapabilityEnabled {
		return deny(GateCapability, ReasonCapabilityDisabled)
	}
	if facts.TransportStatus != TransportOnline && facts.TransportStatus != TransportDegraded {
		return deny(GateTransport, ReasonTransportOffline)
	}
	if !facts.TransportSupports {
		return deny(GateTransport, ReasonTransportUnsupported)
	}
	if !facts.TargetFound {
		return deny(GateTarget, ReasonTargetNotFound)
	}
	if facts.TargetBlocked {
		return deny(GateTarget, ReasonTargetBlocked)
	}
	if facts.TargetCapability != EffectAllow {
		return deny(GateTargetCapability, ReasonTargetCapabilityDenied)
	}
	if facts.TargetAction != EffectAllow {
		// Distinct reason: the capability-level access row allowed the chat,
		// but the row for this specific action is missing or deny. Sharing
		// one reason here made "which row is wrong" undiagnosable from logs.
		return deny(GateTargetCapability, ReasonTargetActionDenied)
	}

	if req.RouteID != "" {
		if !facts.RouteFound || !facts.RouteEnabled {
			return deny(GateRoute, ReasonRouteInactive)
		}
		if facts.RouteTarget != req.Target {
			return deny(GateRoute, ReasonRouteTargetMismatch)
		}
	}
	if req.RequestID != "" {
		if !facts.PendingRequestFound {
			return deny(GatePendingRequest, ReasonPendingRequestExpired)
		}
		if facts.PendingRequestTarget != req.Target {
			return deny(GatePendingRequest, ReasonPendingRequestActorDenied)
		}
	}

	if req.Action.RequiresActor() {
		if req.ActorID == "" {
			return deny(GateActor, ReasonActorRequired)
		}
		if req.Target.Kind == TargetDirectChat {
			if facts.PeerActorID == "" || facts.PeerActorID != req.ActorID {
				return deny(GateActor, ReasonActorMismatch)
			}
		} else if !facts.ActorRegistered {
			return deny(GateActor, ReasonActorNotRegistered)
		}
		if facts.ActorAction != EffectAllow {
			reason := ReasonActorActionDenied
			if req.RequestID != "" {
				reason = ReasonPendingRequestActorDenied
			}
			return deny(GateActorAction, reason)
		}
	}

	return AuthorizationDecision{Allowed: true, Reason: ReasonAllowed, Facts: facts}
}

type Evaluator struct{ source FactSource }

func NewEvaluator(source FactSource) *Evaluator { return &Evaluator{source: source} }

func (e *Evaluator) Evaluate(ctx context.Context, req AuthorizationRequest) AuthorizationDecision {
	if e == nil || e.source == nil {
		return AuthorizationDecision{Reason: ReasonEvaluationFailed}
	}
	facts, err := e.source.Snapshot(ctx, req)
	if err != nil {
		return AuthorizationDecision{Reason: ReasonEvaluationFailed}
	}
	return Evaluate(req, facts)
}
