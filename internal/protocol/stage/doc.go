// Package stage defines transport-independent protocol endpoints and ordered
// endpoint wrappers.
//
// The package is the additive foundation for the Protocol Stage Chain design in
// .design/protocol-stage-chain.md. It intentionally has no integration with the
// current server dispatch path yet: adding these contracts must not change
// production traffic until bridges and feature stages are validated separately.
package stage
