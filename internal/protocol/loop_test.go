package protocol

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// closeNotifyRecorder adds CloseNotify to httptest.ResponseRecorder so it can
// back a gin.ResponseWriter driven by RunLoop.
type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
}

func (r *closeNotifyRecorder) CloseNotify() <-chan bool { return make(chan bool) }

// TestMarkFirstToken_RecordsOnceEarliestWins verifies the single source of
// truth is idempotent: the first signal wins and later calls never overwrite.
func TestMarkFirstToken_RecordsOnceEarliestWins(t *testing.T) {
	c := &gin.Context{}

	_, exists := c.Get(constant.CtxKeyFirstTokenTime)
	assert.False(t, exists)

	MarkFirstToken(c)
	v, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok)
	first := v.(time.Time)

	time.Sleep(5 * time.Millisecond)
	MarkFirstToken(c)
	v2, _ := c.Get(constant.CtxKeyFirstTokenTime)
	assert.Equal(t, first, v2.(time.Time), "later mark must not overwrite earliest signal")
}

// TestCommitFirstChunk_RecordsFirstToken verifies the commit seam records TTFT
// even when no failover gate is installed on the writer.
func TestCommitFirstChunk_RecordsFirstToken(t *testing.T) {
	c := &gin.Context{}

	CommitFirstChunk(c)

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "CommitFirstChunk must record the first-token time")
}

// TestRunLoop_RecordsFirstTokenOnFirstChunk exercises the real streaming wiring:
// RunLoop -> commitFirstChunk -> MarkFirstToken. Before any chunk the first-token
// time is unset; once the first chunk is produced it is recorded.
func TestRunLoop_RecordsFirstTokenOnFirstChunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)

	_, exists := c.Get(constant.CtxKeyFirstTokenTime)
	assert.False(t, exists, "no first-token time before streaming starts")

	calls := 0
	RunLoop(c, func(wr io.Writer) bool {
		calls++
		if calls == 1 {
			_, _ = wr.Write([]byte("data: hi\n\n"))
			return true // first chunk produced
		}
		return false // stop
	})

	_, ok := c.Get(constant.CtxKeyFirstTokenTime)
	assert.True(t, ok, "RunLoop must record the first-token time after the first chunk")
}
