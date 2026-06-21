package vmodelclient

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/vmodel"
)

// injectedPreContentError reproduces a virtual model's pre-content error
// injection (e.g. the "virtual-fail-500" mock) as a Go error, so the in-process
// vmodel clients fail exactly like the HTTP virtualserver's writePreContentError*
// path. Without this an error model would silently return a normal 200 on the
// in-process path, and mid-request failover could never trigger.
//
// The returned error maps to HTTP 500 via upstreamForwardStatus's default, which
// is retryable. Status-specific fidelity (e.g. surfacing 429 vs 500 to the
// health monitor) is not reproduced in-process; only the failure itself is.
// Returns nil when the model declares no pre-content injection.
func injectedPreContentError(vm any) error {
	e := vmodel.ExtractErrorInjection(vm)
	if e == nil || e.Stage != vmodel.ErrorStagePreContent {
		return nil
	}
	status := e.Status
	if status == 0 {
		status = 500
	}
	msg := e.Message
	if msg == "" {
		msg = "simulated vmodel error"
	}
	return fmt.Errorf("vmodel injected error (status %d): %s", status, msg)
}
