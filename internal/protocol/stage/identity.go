package stage

import (
	"context"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// NewIdentityBridge returns a stateless same-protocol Bridge. It is useful when
// a topology requires an explicit boundary but no wire conversion is needed.
func NewIdentityBridge(api protocol.APIType) Bridge {
	return identityBridge{api: api}
}

type identityBridge struct {
	api protocol.APIType
}

func (b identityBridge) Source() protocol.APIType {
	return b.api
}

func (b identityBridge) Target() protocol.APIType {
	return b.api
}

func (b identityBridge) Capabilities() Capabilities {
	return AllBridgeCapabilities
}

func (b identityBridge) Open(_ context.Context, call Call, _ Operation) (BridgeSession, error) {
	return &identityBridgeSession{call: call}, nil
}

type identityBridgeSession struct {
	call Call
}

func (s *identityBridgeSession) TargetCall() Call {
	return s.call
}

func (s *identityBridgeSession) ConvertComplete(_ context.Context, response *Response) (*Response, error) {
	return response, nil
}

func (s *identityBridgeSession) ConvertStream(_ context.Context, stream EventStream) (EventStream, error) {
	return stream, nil
}

func (s *identityBridgeSession) ConvertError(_ context.Context, err error) error {
	return err
}
