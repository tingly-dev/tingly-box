package protocolserver

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestProtocolStageOriginalInputDisabledDoesNotCaptureBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"large":"payload"}`)

	(&ProtocolHandler{}).rememberProtocolStageOriginalInput(c, typ.ScenarioOpenAI, body)

	if _, exists := c.Get(protocolStageOriginalInputKey); exists {
		t.Fatal("disabled Protocol Stage recording retained the request body")
	}
}

func TestProtocolStageRecordingTracksSetupFailureAfterStageUse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	recording := &protocolStageRequestRecording{}
	enableProtocolStageAttemptTracking(c, recording)

	observeProtocolStageSetupFailure(c, errors.New("ignored before Stage use"))
	recording.mu.Lock()
	if recording.lastAttemptErr != nil {
		t.Fatalf("unused recording captured setup error: %v", recording.lastAttemptErr)
	}
	recording.mu.Unlock()

	recording.observeAttempt(errors.New("first provider failed"))
	observeProtocolStageSetupFailure(c, errors.New("fallback setup failed"))
	recording.mu.Lock()
	defer recording.mu.Unlock()
	if recording.lastAttemptErr == nil || recording.lastAttemptErr.Error() != "fallback setup failed" {
		t.Fatalf("last attempt error = %v", recording.lastAttemptErr)
	}
}
