package agentboot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Internal (in-package) tests for RetryConfig.backoff — unexported, so they
// can't live in the agentboot_test package.

func TestRetryConfigBackoff(t *testing.T) {
	def := DefaultRetryConfig()

	// Exponential growth capped at MaxBackoff.
	assert.Equal(t, 250*time.Millisecond, def.backoff(0))
	assert.Equal(t, 500*time.Millisecond, def.backoff(1))
	assert.Equal(t, time.Second, def.backoff(2))
	assert.Equal(t, 2*time.Second, def.backoff(3))
	assert.Equal(t, 2*time.Second, def.backoff(10))

	// Zero fields fall back to the defaults.
	zero := RetryConfig{}
	assert.Equal(t, 250*time.Millisecond, zero.backoff(0))
	assert.Equal(t, 2*time.Second, zero.backoff(10))
}

func TestRetryPauseRespectsCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.False(t, retryPause(ctx, DefaultRetryConfig(), 0), "a canceled ctx must cut the backoff short")
}

func TestPreOutputFailureProcessErrorFromLaunch(t *testing.T) {
	// The wrapped "start process" error the Runner produces must qualify.
	err := errors.New("start process: exec: not found")
	assert.False(t, PreOutputFailure(err), "a bare non-process error is not retryable")
}
