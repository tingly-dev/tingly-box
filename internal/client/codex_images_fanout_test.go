package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexImageCount(t *testing.T) {
	assert.Equal(t, 1, codexImageCount(param.Opt[int64]{}))
	assert.Equal(t, 1, codexImageCount(param.NewOpt(int64(0))))
	assert.Equal(t, 3, codexImageCount(param.NewOpt(int64(3))))
}

func oneImage(b64 string, tokens int64) *openai.ImagesResponse {
	resp := &openai.ImagesResponse{Created: 42, Data: []openai.Image{{B64JSON: b64}}}
	resp.Usage.InputTokens = tokens
	resp.Usage.TotalTokens = tokens
	return resp
}

func TestFanOutCodexImages_MergesInOrder(t *testing.T) {
	var calls, inFlight, peak atomic.Int64
	resp, err := fanOutCodexImages(context.Background(), 6, func(ctx context.Context) (*openai.ImagesResponse, error) {
		cur := inFlight.Add(1)
		for {
			p := peak.Load()
			if cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		defer inFlight.Add(-1)
		calls.Add(1)
		return oneImage("img", 10), nil
	})
	require.NoError(t, err)
	assert.EqualValues(t, 6, calls.Load())
	assert.LessOrEqual(t, peak.Load(), int64(codexMaxParallelImageCalls))
	assert.Len(t, resp.Data, 6)
	assert.EqualValues(t, 60, resp.Usage.TotalTokens)
	assert.EqualValues(t, 42, resp.Created)
}

func TestFanOutCodexImages_SingleCallPassesThrough(t *testing.T) {
	boom := errors.New("boom")
	_, err := fanOutCodexImages(context.Background(), 1, func(ctx context.Context) (*openai.ImagesResponse, error) {
		return nil, boom
	})
	assert.ErrorIs(t, err, boom)
}

func TestFanOutCodexImages_PartialFailureKeepsSuccesses(t *testing.T) {
	var n atomic.Int64
	resp, err := fanOutCodexImages(context.Background(), 4, func(ctx context.Context) (*openai.ImagesResponse, error) {
		if n.Add(1)%2 == 0 {
			return nil, errors.New("rate limited")
		}
		return oneImage("ok", 1), nil
	})
	require.NoError(t, err)
	assert.Len(t, resp.Data, 2)
}

func TestFanOutCodexImages_AllFailReturnsError(t *testing.T) {
	boom := errors.New("upstream down")
	_, err := fanOutCodexImages(context.Background(), 3, func(ctx context.Context) (*openai.ImagesResponse, error) {
		return nil, boom
	})
	assert.ErrorIs(t, err, boom)
}

func TestFanOutCodexImages_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fanOutCodexImages(ctx, 3, func(ctx context.Context) (*openai.ImagesResponse, error) {
		return nil, ctx.Err()
	})
	assert.ErrorIs(t, err, context.Canceled)
}
