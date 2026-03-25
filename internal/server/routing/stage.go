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
	Evaluate(ctx *SelectionContext) (*SelectionResult, bool)
}

// PipelineMode defines different pipeline configurations
type PipelineMode string

const (
	// PipelineModeGlobalAffinity: Affinity → Smart Routing → Load Balancer
	// Session-locked service bypasses smart routing evaluation
	PipelineModeGlobalAffinity PipelineMode = "global_affinity"

	// PipelineModeSmartAffinity: Smart Routing → Affinity → Load Balancer
	// Smart routing evaluates first, then affinity locks within matched rule
	PipelineModeSmartAffinity PipelineMode = "smart_affinity"

	// PipelineModeNoAffinity: Smart Routing → Load Balancer
	// No session locking, every request re-evaluates routing
	PipelineModeNoAffinity PipelineMode = "no_affinity"
)
