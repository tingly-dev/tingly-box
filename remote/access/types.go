// Package access defines the product authorization vocabulary shared by the
// bot runtime, persistence adapters, and control-plane APIs.
package access

import "context"

type CapabilityName string

const (
	CapabilityNotify        CapabilityName = "notify"
	CapabilityRemoteControl CapabilityName = "remote_control"
)

func (c CapabilityName) Valid() bool {
	return c == CapabilityNotify || c == CapabilityRemoteControl
}

type ActionName string

const (
	ActionAccess                  ActionName = "access"
	ActionNotifyReceive           ActionName = "notify.receive"
	ActionNotifyReply             ActionName = "notify.reply"
	ActionRemoteControlStart      ActionName = "remote_control.start"
	ActionRemoteControlApprove    ActionName = "remote_control.approve"
	ActionRemoteControlPrivileged ActionName = "remote_control.privileged"
)

func (a ActionName) Capability() (CapabilityName, bool) {
	switch a {
	case ActionNotifyReceive, ActionNotifyReply:
		return CapabilityNotify, true
	case ActionRemoteControlStart, ActionRemoteControlApprove, ActionRemoteControlPrivileged:
		return CapabilityRemoteControl, true
	default:
		return "", false
	}
}

func (a ActionName) RequiresActor() bool { return a != ActionNotifyReceive }

type AccessEffect string

const (
	EffectAllow AccessEffect = "allow"
	EffectDeny  AccessEffect = "deny"
)

type TargetKind string

const (
	TargetDirectChat TargetKind = "direct_chat"
	TargetGroup      TargetKind = "group"
)

type TargetRef struct {
	Kind TargetKind `json:"kind"`
	ID   string     `json:"id"`
}

type TransportStatus string

const (
	TransportConnecting TransportStatus = "connecting"
	TransportOnline     TransportStatus = "online"
	TransportDegraded   TransportStatus = "degraded"
	TransportOffline    TransportStatus = "offline"
)

type GateName string

const (
	GateBot              GateName = "bot"
	GateCapability       GateName = "capability"
	GateTransport        GateName = "transport"
	GateTarget           GateName = "target"
	GateTargetCapability GateName = "target_capability"
	GateActor            GateName = "actor"
	GateActorAction      GateName = "actor_action"
	GateRoute            GateName = "route"
	GatePendingRequest   GateName = "pending_request"
)

type DecisionReason string

const (
	ReasonAllowed                   DecisionReason = "allowed"
	ReasonBotDisabled               DecisionReason = "bot_disabled"
	ReasonCapabilityDisabled        DecisionReason = "capability_disabled"
	ReasonTransportOffline          DecisionReason = "transport_offline"
	ReasonTransportUnsupported      DecisionReason = "transport_unsupported"
	ReasonTargetNotFound            DecisionReason = "target_not_found"
	ReasonTargetBlocked             DecisionReason = "target_blocked"
	ReasonTargetCapabilityDenied    DecisionReason = "target_capability_denied"
	ReasonTargetActionDenied        DecisionReason = "target_action_denied"
	ReasonActorRequired             DecisionReason = "actor_required"
	ReasonActorMismatch             DecisionReason = "actor_mismatch"
	ReasonActorNotRegistered        DecisionReason = "actor_not_registered"
	ReasonActorActionDenied         DecisionReason = "actor_action_denied"
	ReasonRouteInactive             DecisionReason = "route_inactive"
	ReasonRouteTargetMismatch       DecisionReason = "route_target_mismatch"
	ReasonPendingRequestExpired     DecisionReason = "pending_request_expired"
	ReasonPendingRequestActorDenied DecisionReason = "pending_request_actor_denied"
	ReasonEvaluationFailed          DecisionReason = "evaluation_failed"
)

type AuthorizationRequest struct {
	BotUUID    string         `json:"bot_uuid"`
	Target     TargetRef      `json:"target"`
	ActorID    string         `json:"actor_id,omitempty"`
	Capability CapabilityName `json:"capability"`
	Action     ActionName     `json:"action"`
	RouteID    string         `json:"route_id,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
}

type DecisionFacts struct {
	BotEnabled           bool            `json:"bot_enabled"`
	CapabilityEnabled    bool            `json:"capability_enabled"`
	TransportStatus      TransportStatus `json:"transport_status"`
	TransportSupports    bool            `json:"transport_supports"`
	TargetFound          bool            `json:"target_found"`
	TargetBlocked        bool            `json:"target_blocked"`
	TargetCapability     AccessEffect    `json:"target_capability"`
	TargetAction         AccessEffect    `json:"target_action,omitempty"`
	PeerActorID          string          `json:"peer_actor_id,omitempty"`
	ActorRegistered      bool            `json:"actor_registered,omitempty"`
	ActorAction          AccessEffect    `json:"actor_action,omitempty"`
	RouteFound           bool            `json:"route_found,omitempty"`
	RouteEnabled         bool            `json:"route_enabled,omitempty"`
	RouteTarget          TargetRef       `json:"route_target,omitempty"`
	PendingRequestFound  bool            `json:"pending_request_found,omitempty"`
	PendingRequestTarget TargetRef       `json:"pending_request_target,omitempty"`
}

type AuthorizationDecision struct {
	Allowed    bool           `json:"allowed"`
	Reason     DecisionReason `json:"reason"`
	FailedGate GateName       `json:"failed_gate,omitempty"`
	Facts      DecisionFacts  `json:"facts"`
}

// FactSource loads one internally consistent authorization snapshot. Store
// implementations should use a read transaction when facts span tables.
type FactSource interface {
	Snapshot(context.Context, AuthorizationRequest) (DecisionFacts, error)
}

type Authorizer interface {
	Evaluate(context.Context, AuthorizationRequest) AuthorizationDecision
}
