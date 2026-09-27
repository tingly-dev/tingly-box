package client

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/sirupsen/logrus"
)

// Multi-image requests (n > 1) on Codex.
//
// None of the Codex image surfaces returns more than one image per call: the
// Responses image_generation tool produces a single image_generation_call, and
// the native images/edits endpoint, although its schema carries `n`, is only
// ever called with one by the Codex CLI. So an `n` is served here by issuing n
// single-image calls in parallel and merging their data[] — the caller asked
// for n images and gets n, whichever surface serves them, instead of one image
// and a debug log.
//
// See .design/image-mask.md §9.

// codexMaxParallelImageCalls bounds how many single-image calls one request
// keeps in flight. Each is a full image generation on the user's subscription,
// so an unbounded burst mostly buys rate-limit errors; a small window keeps
// wall time close to one call for the common n (the Playground caps it at 10).
const codexMaxParallelImageCalls = 4

// codexImageCount reads the requested image count, treating unset/invalid as 1.
func codexImageCount(n param.Opt[int64]) int {
	if !n.Valid() || n.Value < 1 {
		return 1
	}
	return int(n.Value)
}

// ImageCallFailures collects why individual calls of a multi-image request
// failed when the request as a whole still succeeded. The client interface
// returns one *openai.ImagesResponse and one error, and "error" means the
// request failed — so a partial failure has no place in that shape. The server
// layer installs a collector on the request context instead, and after a
// successful call reads it to tell the caller why some images are missing
// (see .design/image-mask.md §9.1).
type ImageCallFailures struct {
	mu   sync.Mutex
	errs []error
}

type imageCallFailuresKey struct{}

// WithImageCallFailures returns ctx carrying a fresh collector, and the
// collector to read after the call returns.
func WithImageCallFailures(ctx context.Context) (context.Context, *ImageCallFailures) {
	f := &ImageCallFailures{}
	return context.WithValue(ctx, imageCallFailuresKey{}, f), f
}

// Errors returns the recorded per-call failures, in call order.
func (f *ImageCallFailures) Errors() []error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]error(nil), f.errs...)
}

// Add records failed calls. Safe for concurrent use and on a nil collector.
func (f *ImageCallFailures) Add(errs ...error) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, errs...)
}

func imageCallFailuresFrom(ctx context.Context) *ImageCallFailures {
	f, _ := ctx.Value(imageCallFailuresKey{}).(*ImageCallFailures)
	return f
}

// fanOutCodexImages runs `one` n times (at most codexMaxParallelImageCalls at
// once) and merges the results in call order.
//
// Partial failure returns what succeeded: throwing away finished generations
// that were already paid for would be the worse outcome. That includes the
// request deadline firing while a later wave is still running — the finished
// waves are kept. Why the other calls failed goes to the context's
// ImageCallFailures collector, so the caller is told, not just shown fewer
// images. Only when every call fails is the request an error.
func fanOutCodexImages(ctx context.Context, n int, one func(ctx context.Context) (*openai.ImagesResponse, error)) (*openai.ImagesResponse, error) {
	if n <= 1 {
		return one(ctx)
	}

	results := make([]*openai.ImagesResponse, n)
	errs := make([]error, n)
	sem := make(chan struct{}, codexMaxParallelImageCalls)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			results[i], errs[i] = one(ctx)
		}(i)
	}
	wg.Wait()

	merged := &openai.ImagesResponse{}
	var failures, callErrs []error
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			failures = append(failures, fmt.Errorf("image %d/%d: %w", i+1, n, errs[i]))
			callErrs = append(callErrs, errs[i])
			continue
		}
		mergeCodexImagesResponse(merged, results[i])
	}

	if len(merged.Data) == 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(failures) == 0 {
			return nil, fmt.Errorf("codex returned no image data for %d parallel calls", n)
		}
		return nil, failures[0]
	}
	if len(failures) > 0 {
		imageCallFailuresFrom(ctx).Add(callErrs...)
		logrus.WithContext(ctx).Warnf("[Codex] %d of %d parallel image calls failed, returning %d images: %v",
			len(failures), n, len(merged.Data), errors.Join(failures...))
	} else {
		logrus.WithContext(ctx).Infof("[Codex] Merged %d parallel image calls, images: %d", n, len(merged.Data))
	}
	return merged, nil
}

// mergeCodexImagesResponse appends one single-image response to the merged
// result: images in order, usage summed, and the scalar echo fields
// (created/background/size/...) taken from the first response that has them.
func mergeCodexImagesResponse(dst, src *openai.ImagesResponse) {
	if src == nil {
		return
	}
	dst.Data = append(dst.Data, src.Data...)
	if dst.Created == 0 {
		dst.Created = src.Created
	}
	if dst.Background == "" {
		dst.Background = src.Background
	}
	if dst.OutputFormat == "" {
		dst.OutputFormat = src.OutputFormat
	}
	if dst.Quality == "" {
		dst.Quality = src.Quality
	}
	if dst.Size == "" {
		dst.Size = src.Size
	}

	u, s := &dst.Usage, src.Usage
	u.InputTokens += s.InputTokens
	u.OutputTokens += s.OutputTokens
	u.TotalTokens += s.TotalTokens
	u.InputTokensDetails.ImageTokens += s.InputTokensDetails.ImageTokens
	u.InputTokensDetails.TextTokens += s.InputTokensDetails.TextTokens
	u.OutputTokensDetails.ImageTokens += s.OutputTokensDetails.ImageTokens
	u.OutputTokensDetails.TextTokens += s.OutputTokensDetails.TextTokens
}
