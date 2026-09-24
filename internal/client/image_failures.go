package client

import (
	"context"
	"sync"
)

// ImageFailure is one image of a multi-image request that failed while its
// siblings succeeded.
type ImageFailure struct {
	Index     int // 1-based position among the requested images
	Requested int
	Err       error
}

// ImageFailures collects the per-image failures of a request that still
// returned images. fanOutCodexImages keeps what succeeded and answers 200, so
// without this the only trace of a dropped image (moderation block, rate
// limit) was a Warn line in the Logs page and the caller silently got fewer
// images than it asked for. The handler installs it via WithImageFailures and
// reports its contents in the response.
type ImageFailures struct {
	mu       sync.Mutex
	failures []ImageFailure
}

type imageFailuresKey struct{}

// WithImageFailures returns a context whose image calls report partial
// failures into the returned collector.
func WithImageFailures(ctx context.Context) (context.Context, *ImageFailures) {
	f := &ImageFailures{}
	return context.WithValue(ctx, imageFailuresKey{}, f), f
}

// All returns the failures recorded so far.
func (f *ImageFailures) All() []ImageFailure {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ImageFailure(nil), f.failures...)
}

// Add records failures.
func (f *ImageFailures) Add(failures ...ImageFailure) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures = append(f.failures, failures...)
}

// reportImageFailures records failures on ctx's collector, if any.
func reportImageFailures(ctx context.Context, failures []ImageFailure) {
	if f, _ := ctx.Value(imageFailuresKey{}).(*ImageFailures); f != nil && len(failures) > 0 {
		f.Add(failures...)
	}
}
