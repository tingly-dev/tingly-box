package vmodel

import (
	"context"
	"time"
)

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

// EmitChunks returns the
// context error if cancellation happens while waiting or if the emit callback
// stops accepting chunks.
func EmitChunks(ctx context.Context, chunks []string, perChunkDelay time.Duration, emit func(index int, chunk string) bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for i, chunk := range chunks {
		if perChunkDelay > 0 {
			timer := time.NewTimer(perChunkDelay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if !emit(i, chunk) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return context.Canceled
		}
	}
	return nil
}
