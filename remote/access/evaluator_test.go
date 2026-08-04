package access

import "testing"

func allowedFacts() DecisionFacts {
	return DecisionFacts{
		BotEnabled: true, CapabilityEnabled: true,
		TransportStatus: TransportOnline, TransportSupports: true,
		TargetFound: true, TargetCapability: EffectAllow, TargetAction: EffectAllow,
		PeerActorID: "alice", ActorRegistered: true, ActorAction: EffectAllow,
	}
}

func TestEvaluateDecisionOrder(t *testing.T) {
	baseReq := AuthorizationRequest{
		BotUUID: "bot-1", Target: TargetRef{Kind: TargetDirectChat, ID: "chat-1"},
		ActorID: "alice", Capability: CapabilityRemoteControl, Action: ActionRemoteControlStart,
	}
	tests := []struct {
		name   string
		mutate func(*DecisionFacts)
		reason DecisionReason
		gate   GateName
	}{
		{"bot", func(f *DecisionFacts) { f.BotEnabled = false }, ReasonBotDisabled, GateBot},
		{"capability", func(f *DecisionFacts) { f.CapabilityEnabled = false }, ReasonCapabilityDisabled, GateCapability},
		{"transport offline", func(f *DecisionFacts) { f.TransportStatus = TransportOffline }, ReasonTransportOffline, GateTransport},
		{"transport unsupported", func(f *DecisionFacts) { f.TransportSupports = false }, ReasonTransportUnsupported, GateTransport},
		{"target missing", func(f *DecisionFacts) { f.TargetFound = false }, ReasonTargetNotFound, GateTarget},
		{"target blocked", func(f *DecisionFacts) { f.TargetBlocked = true }, ReasonTargetBlocked, GateTarget},
		{"target access missing", func(f *DecisionFacts) { f.TargetCapability = "" }, ReasonTargetCapabilityDenied, GateTargetCapability},
		{"target action denied", func(f *DecisionFacts) { f.TargetAction = EffectDeny }, ReasonTargetActionDenied, GateTargetCapability},
		{"peer mismatch", func(f *DecisionFacts) { f.PeerActorID = "bob" }, ReasonActorMismatch, GateActor},
		{"action denied", func(f *DecisionFacts) { f.ActorAction = EffectDeny }, ReasonActorActionDenied, GateActorAction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := allowedFacts()
			tt.mutate(&facts)
			got := Evaluate(baseReq, facts)
			if got.Allowed || got.Reason != tt.reason || got.FailedGate != tt.gate {
				t.Fatalf("Evaluate() = allowed=%v reason=%q gate=%q", got.Allowed, got.Reason, got.FailedGate)
			}
		})
	}
}

func TestEvaluateGroupAndActorlessNotify(t *testing.T) {
	group := AuthorizationRequest{Target: TargetRef{Kind: TargetGroup, ID: "g"}, ActorID: "alice", Capability: CapabilityRemoteControl, Action: ActionRemoteControlStart}
	facts := allowedFacts()
	facts.ActorRegistered = false
	if got := Evaluate(group, facts); got.Reason != ReasonActorNotRegistered {
		t.Fatalf("unbound group actor reason = %q", got.Reason)
	}

	notify := AuthorizationRequest{Target: TargetRef{Kind: TargetGroup, ID: "g"}, Capability: CapabilityNotify, Action: ActionNotifyReceive}
	facts.ActorAction = ""
	if got := Evaluate(notify, facts); !got.Allowed {
		t.Fatalf("actorless notify denied: %#v", got)
	}
}
