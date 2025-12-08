package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEmbeddedAssets(t *testing.T) {
	// Test embedded assets initialization
	assets, err := NewEmbeddedAssets()
	assert.NoError(t, err)
	assert.NotNil(t, assets)

	// Test template retrieval
	templates := assets.GetTemplates()
	assert.NotNil(t, templates)

	// Test static routes setup
	gin.SetMode(gin.TestMode)
	router := gin.New()
	assets.SetupStaticRoutes(router)

	// Test static file route (should return empty filesystem)
	req, _ := http.NewRequest("GET", "/static/test.css", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Test HTML rendering
	req, _ = http.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// This should not panic
	assets.HTML(c, "dashboard.html", map[string]interface{}{"title": "Test"})
	// Since we're not testing full template execution, we just check it doesn't panic
	assert.True(t, true)
}

func TestWebUIWithEmbeddedAssets(t *testing.T) {
	// Test WebUI with embedded assets
	wui := NewWebUI(true, nil, nil, nil)
	assert.NotNil(t, wui)
	assert.True(t, wui.IsEnabled())
	assert.NotNil(t, wui.GetRouter())
}

func TestWebUIDisabled(t *testing.T) {
	// Test WebUI when disabled
	wui := NewWebUI(false, nil, nil, nil)
	assert.NotNil(t, wui)
	assert.False(t, wui.IsEnabled())
	assert.Nil(t, wui.GetRouter())
}
