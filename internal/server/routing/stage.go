package routing

// SelectionStage represents a single stage in the service selection pipeline.
// Each stage can evaluate the context and either:
// - Return a service selection (result, true)
// - Pass to the next stage (nil, false)
type SelectionStage interface {
	// Name returns the stage identifier for logging and metrics
	Name() string

	// Evaluate attempts to select a service based on the context.
	// Returns:
	//   - (result, true) if this stage selected a service (stops pipeline)
	//   - (nil, false) if this stage cannot select (continue to next stage)
	Evaluate(ctx *SelectionContext, state *selectionState) (*SelectionResult, bool)
}
