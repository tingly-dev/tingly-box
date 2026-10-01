package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func postOpenURL(t *testing.T, auth, body string, open func(string) error) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/gui/open-url", openURLHandler(func() string { return "tok" }, open))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gui/open-url", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec.Code
}

func TestOpenURLOpensWebLinks(t *testing.T) {
	var opened []string
	open := func(u string) error { opened = append(opened, u); return nil }
	for _, u := range []string{
		"https://github.com/tingly-dev/tingly-box",
		"http://example.com/a?b=c",
		"mailto:box@tingly.dev",
	} {
		if code := postOpenURL(t, "Bearer tok", `{"url":"`+u+`"}`, open); code != http.StatusOK {
			t.Errorf("%s: got %d, want 200", u, code)
		}
	}
	if len(opened) != 3 {
		t.Errorf("opened %v, want all three", opened)
	}
}

func TestOpenURLRefusesOtherSchemes(t *testing.T) {
	open := func(u string) error { t.Errorf("opened %q", u); return nil }
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "vscode://x", "https://", "relative/path", ""} {
		if code := postOpenURL(t, "Bearer tok", `{"url":"`+u+`"}`, open); code != http.StatusBadRequest {
			t.Errorf("%q: got %d, want 400", u, code)
		}
	}
}

func TestOpenURLRejectsWrongToken(t *testing.T) {
	open := func(u string) error { t.Errorf("opened %q without a valid token", u); return nil }
	for _, auth := range []string{"", "Bearer nope"} {
		if code := postOpenURL(t, auth, `{"url":"https://example.com"}`, open); code != http.StatusForbidden {
			t.Errorf("auth %q: got %d, want 403", auth, code)
		}
	}
}
