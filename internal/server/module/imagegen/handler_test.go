package imagegen

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

// A real 1x1 PNG, so content sniffing sees an image.
var onePixelPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	manager := swagger.NewRouteManager(engine)
	group := manager.NewGroup("api", "v1", "")
	NewHandler(t.TempDir()).RegisterRoutes(&module.Routes{V1: group})
	return engine
}

func do(t *testing.T, r http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestImportWorkbenchAndRootProtection(t *testing.T) {
	r := newTestRouter(t)

	w, out := do(t, r, http.MethodPost, "/api/v1/imagegen/images",
		map[string]string{"data": "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG), "name": "mira.png"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rootID := out["image"].(map[string]any)["id"].(string)

	w, _ = do(t, r, http.MethodPost, "/api/v1/imagegen/images", map[string]string{"data": base64.StdEncoding.EncodeToString([]byte("hello"))})
	assert.Equal(t, http.StatusBadRequest, w.Code, "non-image data is refused")

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/imagegen/images/"+rootID+"/file", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, onePixelPNG, w.Body.Bytes())

	w, out = do(t, r, http.MethodPost, "/api/v1/imagegen/workbenches",
		map[string]string{"name": "Mira", "description": "red scarf", "root_image_id": rootID})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	wbID := out["workbench"].(map[string]any)["id"].(string)

	w, _ = do(t, r, http.MethodDelete, "/api/v1/imagegen/images/"+rootID, nil)
	assert.Equal(t, http.StatusConflict, w.Code, "a workbench root cannot be deleted")

	w, out = do(t, r, http.MethodGet, "/api/v1/imagegen/images", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, out["images"], 1)

	w, _ = do(t, r, http.MethodDelete, "/api/v1/imagegen/workbenches/"+wbID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = do(t, r, http.MethodDelete, "/api/v1/imagegen/images/"+rootID, nil)
	assert.Equal(t, http.StatusOK, w.Code, "once no workbench holds it, the root can go")
}

func TestBadIDsAreRejected(t *testing.T) {
	r := newTestRouter(t)
	w, _ := do(t, r, http.MethodGet, "/api/v1/imagegen/images/..%2F..%2Fetc/file", nil)
	assert.NotEqual(t, http.StatusOK, w.Code)
	w, _ = do(t, r, http.MethodGet, "/api/v1/imagegen/images/not-an-id", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = do(t, r, http.MethodGet, "/api/v1/imagegen/workbenches/nope", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
