// Package stage defines transport-independent protocol endpoints, ordered
// same-protocol stages, and the bidirectional bridges between protocols.
//
// It is the foundation of the Protocol Stage v2 plan in
// .design/protocol-stage-v2.md: provider endpoints, Guardrails and the server
// tool loop become composable levels, and protocol changes happen only in an
// explicit Bridge. Nothing in the server imports this package yet; routes move
// onto it one protocol pair at a time, each cutover deleting the legacy path.
//
// Anthropic V1 is not a chain protocol. V1 is a subset of Anthropic Beta on the
// wire, so V1 requests are upgraded to Beta at the client edge and downgraded
// only where a provider needs the V1 path; every Endpoint, Stage and Bridge in
// a chain speaks anthropic_beta for Anthropic. The contracts enforce this.
//
// Terminal provider endpoints live in stage/upstream; the cross-protocol
// bridges live in stage/anthropicbridge, stage/openaibridge and
// stage/responsesbridge.
package stage
