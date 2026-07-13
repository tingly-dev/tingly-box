package stage

import (
	"context"
	"errors"
	"fmt"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// Bridge describes an immutable, concurrency-safe bidirectional protocol
// adapter. Open converts one source Call to the target protocol and creates all
// mutable response/stream correlation state for that call.
type Bridge interface {
	Source() protocol.APIType
	Target() protocol.APIType
	Capabilities() Capabilities
	Open(ctx context.Context, call Call, operation Operation) (BridgeSession, error)
}

// Operation identifies which endpoint operation the Bridge session is being
// opened for. Request conversion may legitimately differ between complete and
// streaming calls (for example, stream flags and usage options).
type Operation uint8

const (
	OperationComplete Operation = iota + 1
	OperationStream
)

func (o Operation) String() string {
	switch o {
	case OperationComplete:
		return "complete"
	case OperationStream:
		return "stream"
	default:
		return fmt.Sprintf("unknown(%d)", o)
	}
}

// BridgeSession is the per-call reverse path created while converting a request
// inward. A session is used for exactly one Complete or Stream invocation.
//
// ConvertStream must return a source-protocol stream that owns the target stream:
// it converts runtime Next errors and closes the target stream from Close.
type BridgeSession interface {
	TargetCall() Call
	ConvertComplete(ctx context.Context, response *Response) (*Response, error)
	ConvertStream(ctx context.Context, stream EventStream) (EventStream, error)
	ConvertError(ctx context.Context, err error) error
}

// Adapt exposes next in bridge.Source() while calling next in bridge.Target().
// It validates the structural and core-capability boundary without executing a
// request.
func Adapt(next Endpoint, bridge Bridge) (Endpoint, error) {
	if isNil(next) {
		return nil, fmt.Errorf("adapt protocol bridge: target endpoint is nil")
	}
	if isNil(bridge) {
		return nil, fmt.Errorf("adapt protocol bridge: bridge is nil")
	}

	source := bridge.Source()
	if source == "" {
		return nil, fmt.Errorf("adapt protocol bridge: bridge has empty source protocol")
	}
	target := bridge.Target()
	if target == "" {
		return nil, fmt.Errorf("adapt protocol bridge %q -> ?: bridge has empty target protocol", source)
	}
	if next.Protocol() == "" {
		return nil, fmt.Errorf("adapt protocol bridge %q -> %q: target endpoint has empty protocol", source, target)
	}
	if next.Protocol() != target {
		return nil, fmt.Errorf(
			"adapt protocol bridge %q -> %q: cannot call endpoint speaking %q",
			source,
			target,
			next.Protocol(),
		)
	}
	if missing := bridge.Capabilities().Missing(CoreBridgeCapabilities); missing != 0 {
		return nil, fmt.Errorf(
			"adapt protocol bridge %q -> %q: missing core capabilities: %s",
			source,
			target,
			missing,
		)
	}

	return &bridgeEndpoint{
		next:   next,
		bridge: bridge,
		source: source,
		target: target,
	}, nil
}

type bridgeEndpoint struct {
	next   Endpoint
	bridge Bridge
	source protocol.APIType
	target protocol.APIType
}

func (e *bridgeEndpoint) Protocol() protocol.APIType {
	return e.source
}

func (e *bridgeEndpoint) Complete(ctx context.Context, call Call) (*Response, error) {
	session, targetCall, err := e.open(ctx, call, OperationComplete)
	if err != nil {
		return nil, err
	}

	response, err := e.next.Complete(ctx, targetCall)
	if err != nil {
		return nil, e.convertError(ctx, session, err)
	}
	if response == nil {
		return nil, fmt.Errorf("protocol bridge %q -> %q: target endpoint returned a nil response", e.source, e.target)
	}

	converted, err := session.ConvertComplete(ctx, response)
	if err != nil {
		return nil, err
	}
	if converted == nil {
		return nil, fmt.Errorf("protocol bridge %q -> %q: session returned a nil converted response", e.source, e.target)
	}

	result := *converted
	mergeResponseFacts(&result, response)
	return &result, nil
}

func (e *bridgeEndpoint) Stream(ctx context.Context, call Call) (EventStream, error) {
	session, targetCall, err := e.open(ctx, call, OperationStream)
	if err != nil {
		return nil, err
	}

	targetStream, err := e.next.Stream(ctx, targetCall)
	if err != nil {
		return nil, e.convertError(ctx, session, err)
	}
	if isNil(targetStream) {
		return nil, fmt.Errorf("protocol bridge %q -> %q: target endpoint returned a nil stream", e.source, e.target)
	}

	converted, err := session.ConvertStream(ctx, targetStream)
	if err != nil {
		return nil, closeAfterConversionFailure(targetStream, err, e.source, e.target)
	}
	if isNil(converted) {
		err := fmt.Errorf("protocol bridge %q -> %q: session returned a nil converted stream", e.source, e.target)
		return nil, closeAfterConversionFailure(targetStream, err, e.source, e.target)
	}

	return &factPreservingStream{
		converted: converted,
		target:    targetStream,
	}, nil
}

func (e *bridgeEndpoint) open(ctx context.Context, call Call, operation Operation) (BridgeSession, Call, error) {
	session, err := e.bridge.Open(ctx, call, operation)
	if err != nil {
		return nil, Call{}, err
	}
	if isNil(session) {
		return nil, Call{}, fmt.Errorf("protocol bridge %q -> %q: Open returned a nil session", e.source, e.target)
	}

	targetCall := session.TargetCall()
	// Protocol conversion must not erase attempt identity. Any future metadata
	// transformation needs an explicit field and policy rather than a hidden
	// bridge-local mutation.
	targetCall.Metadata = call.Metadata
	return session, targetCall, nil
}

func (e *bridgeEndpoint) convertError(ctx context.Context, session BridgeSession, targetErr error) error {
	converted := session.ConvertError(ctx, targetErr)
	if converted != nil {
		return converted
	}
	return fmt.Errorf(
		"protocol bridge %q -> %q swallowed target error: %w",
		e.source,
		e.target,
		targetErr,
	)
}

func mergeResponseFacts(converted, target *Response) {
	if converted.Usage == nil {
		converted.Usage = target.Usage
	}
	if converted.Model == "" {
		converted.Model = target.Model
	}
	converted.SideEffectsCommitted = converted.SideEffectsCommitted || target.SideEffectsCommitted
}

func mergeStreamFacts(converted, target StreamResult) StreamResult {
	if converted.Usage == nil {
		converted.Usage = target.Usage
	}
	if converted.Model == "" {
		converted.Model = target.Model
	}
	converted.SideEffectsCommitted = converted.SideEffectsCommitted || target.SideEffectsCommitted
	return converted
}

func closeAfterConversionFailure(stream EventStream, conversionErr error, source, target protocol.APIType) error {
	if closeErr := stream.Close(); closeErr != nil {
		return errors.Join(
			conversionErr,
			fmt.Errorf("close target stream after bridge %q -> %q conversion failure: %w", source, target, closeErr),
		)
	}
	return conversionErr
}

// factPreservingStream keeps protocol-neutral facts monotonic while delegating
// event conversion and target-stream ownership to the BridgeSession's stream.
type factPreservingStream struct {
	converted EventStream
	target    EventStream
}

func (s *factPreservingStream) Next(ctx context.Context) (Event, error) {
	return s.converted.Next(ctx)
}

func (s *factPreservingStream) Close() error {
	return s.converted.Close()
}

func (s *factPreservingStream) Result() StreamResult {
	return mergeStreamFacts(s.converted.Result(), s.target.Result())
}
