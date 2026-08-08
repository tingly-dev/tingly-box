package providerquota

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// fakeManager lets each test configure exactly what GetQuota returns per
// provider UUID, without a real store/fetcher.
type fakeManager struct {
	quotas map[string]*quota.ProviderUsage
	errs   map[string]error
}

func (f *fakeManager) GetQuota(_ context.Context, providerUUID string) (*quota.ProviderUsage, error) {
	if err, ok := f.errs[providerUUID]; ok {
		return nil, err
	}
	if u, ok := f.quotas[providerUUID]; ok {
		return u, nil
	}
	return nil, quota.ErrUsageNotFound
}
func (f *fakeManager) GetQuotaNoCache(ctx context.Context, providerUUID string) (*quota.ProviderUsage, error) {
	return f.GetQuota(ctx, providerUUID)
}
func (f *fakeManager) ListQuota(context.Context) ([]*quota.ProviderUsage, error) { return nil, nil }
func (f *fakeManager) Refresh(context.Context) ([]*quota.ProviderUsage, error)   { return nil, nil }
func (f *fakeManager) RefreshProvider(context.Context, string) (*quota.ProviderUsage, error) {
	return nil, nil
}
func (f *fakeManager) Summary(context.Context) (*quota.Summary, error) { return nil, nil }
func (f *fakeManager) IsProviderSupported(string) bool                 { return true }
func (f *fakeManager) StartAutoRefresh(context.Context)                {}
func (f *fakeManager) StopAutoRefresh()                                {}

func postJSON(t *testing.T, h gin.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	h(c)
	return w
}

// TestBatchGetQuota_SkipsProvidersWithNoData is the regression test for the
// bug e2e testing surfaced: a provider with no quota data (e.g. a vmodel/
// local provider with no registered fetcher) used to 500 the WHOLE batch
// request instead of just being omitted from the result — because
// Manager.GetQuota re-wrapped ErrUsageNotFound into a new error, breaking
// the handler's `err != quota.ErrUsageNotFound` identity check (fixed in
// ai/quota/manager.go). This test exercises the real (non-fake) comparison
// path in the handler with a manager that returns the sentinel directly.
func TestBatchGetQuota_SkipsProvidersWithNoData(t *testing.T) {
	mgr := &fakeManager{
		quotas: map[string]*quota.ProviderUsage{
			"has-data": {ProviderUUID: "has-data", ProviderName: "Real"},
		},
		// "no-data" provider: absent from both maps -> ErrUsageNotFound.
	}
	h := NewHandler(mgr, logrus.StandardLogger())

	w := postJSON(t, h.BatchGetQuota, BatchGetQuotaRequest{ProviderUUIDs: []string{"has-data", "no-data"}})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp BatchGetQuotaResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp.Data["has-data"]; !ok {
		t.Fatalf("expected has-data in response, got %+v", resp.Data)
	}
	if _, ok := resp.Data["no-data"]; ok {
		t.Fatalf("expected no-data to be omitted (not errored), got %+v", resp.Data)
	}
}

// TestBatchGetQuota_FailsOnlyWhenEveryProviderErrors confirms a genuine
// (non-not-found) error still surfaces when NO provider in the batch
// produced usable data — distinct from the not-found-is-a-skip case above.
func TestBatchGetQuota_FailsOnlyWhenEveryProviderErrors(t *testing.T) {
	mgr := &fakeManager{
		errs: map[string]error{"broken": context.DeadlineExceeded},
	}
	h := NewHandler(mgr, logrus.StandardLogger())

	w := postJSON(t, h.BatchGetQuota, BatchGetQuotaRequest{ProviderUUIDs: []string{"broken"}})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
}

// TestGetQuota_NotFoundReturns404 pins the single-provider GetQuota's
// not-found response, the same sentinel path BatchGetQuota relies on.
func TestGetQuota_NotFoundReturns404(t *testing.T) {
	mgr := &fakeManager{}
	h := NewHandler(mgr, logrus.StandardLogger())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/provider-quota/no-data", nil)
	c.Params = gin.Params{{Key: "uuid", Value: "no-data"}}
	h.GetQuota(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}
