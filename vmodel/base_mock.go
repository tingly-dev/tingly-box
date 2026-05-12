package vmodel

import "time"

// DefaultMockOwnedBy is the OwnedBy value reported by every built-in mock
// virtual model in the OpenAI-compatible models list.
const DefaultMockOwnedBy = "tingly-box-virtual"

// DefaultMockDescription is the description returned by mock virtual models
// when their config does not provide an explicit one.
const DefaultMockDescription = "A virtual model that returns fixed responses for testing"

// BaseMockModel is the protocol-neutral half of an in-memory mock virtual
// model. Protocol-specific mocks embed it and only need to add their own
// Handle*/Handle*Stream methods. All identity / metadata methods of the
// VirtualModel interface live here.
type BaseMockModel struct {
	ID          string
	Name        string
	Description string
	Type        VirtualModelType
	Delay       time.Duration
}

func (b *BaseMockModel) GetID() string                 { return b.ID }
func (b *BaseMockModel) GetName() string               { return b.Name }
func (b *BaseMockModel) GetDescription() string        { return b.Description }
func (b *BaseMockModel) GetType() VirtualModelType     { return b.Type }
func (b *BaseMockModel) SimulatedDelay() time.Duration { return b.Delay }

// ToModel returns the OpenAI-compatible models-list entry for this mock.
func (b *BaseMockModel) ToModel() Model {
	return Model{
		ID:      b.ID,
		Object:  "model",
		Created: 0,
		OwnedBy: DefaultMockOwnedBy,
	}
}
