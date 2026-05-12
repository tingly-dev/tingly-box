package vmodel

import "time"

// DefaultStreamChunkDelay is the per-chunk sleep used by stream helpers when
// no explicit total simulated delay is configured.
const DefaultStreamChunkDelay = 50 * time.Millisecond

// ResolveChunkDelay computes the per-chunk sleep duration for a stream.
//   - if totalDelay > 0 and chunkCount > 0 → totalDelay / chunkCount
//   - otherwise → DefaultStreamChunkDelay
//
// This is the shared latency-distribution rule used by all mock streams across
// protocols.
func ResolveChunkDelay(totalDelay time.Duration, chunkCount int) time.Duration {
	if totalDelay > 0 && chunkCount > 0 {
		return totalDelay / time.Duration(chunkCount)
	}
	return DefaultStreamChunkDelay
}

// EmitChunks invokes emit for every chunk in order, sleeping perChunkDelay
// before each emission. It is the protocol-neutral inner loop shared by mock
// streams. Callers construct their own protocol-specific event types inside
// the emit closure.
func EmitChunks(chunks []string, perChunkDelay time.Duration, emit func(index int, chunk string)) {
	for i, chunk := range chunks {
		if perChunkDelay > 0 {
			time.Sleep(perChunkDelay)
		}
		emit(i, chunk)
	}
}
