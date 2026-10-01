package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSaveFileEngine(prompt func(name string) (string, error)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/gui/save", saveFileHandler(func() string { return "tok" }, prompt))
	return engine
}

func postSave(engine *gin.Engine, name, auth string, body []byte) (*httptest.ResponseRecorder, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gui/save?name="+name, bytes.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestSaveFileWritesWhereTheUserChose(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "slices.zip")
	var suggested string
	engine := newSaveFileEngine(func(name string) (string, error) {
		suggested = name
		return dest, nil
	})

	rec, out := postSave(engine, "slices.zip", "Bearer tok", []byte("payload"))
	if rec.Code != http.StatusOK || out["saved"] != true {
		t.Fatalf("got %d %v, want 200 saved", rec.Code, out)
	}
	if suggested != "slices.zip" {
		t.Errorf("dialog suggested %q, want slices.zip", suggested)
	}
	if got, _ := os.ReadFile(dest); string(got) != "payload" {
		t.Errorf("file holds %q, want payload", got)
	}
}

func TestSaveFileCancelIsNotAnError(t *testing.T) {
	engine := newSaveFileEngine(func(string) (string, error) { return "", nil })
	rec, out := postSave(engine, "a.txt", "Bearer tok", []byte("x"))
	if rec.Code != http.StatusOK || out["saved"] != false {
		t.Fatalf("got %d %v, want 200 saved=false", rec.Code, out)
	}
}

func TestSaveFileRejectsWrongToken(t *testing.T) {
	called := false
	engine := newSaveFileEngine(func(string) (string, error) { called = true; return "", nil })
	for _, auth := range []string{"", "Bearer nope"} {
		rec, _ := postSave(engine, "a.txt", auth, []byte("x"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("auth %q: got %d, want 403", auth, rec.Code)
		}
	}
	if called {
		t.Error("dialog shown without a valid token")
	}
}

func TestSaveFileDialogError(t *testing.T) {
	engine := newSaveFileEngine(func(string) (string, error) { return "", errors.New("no display") })
	rec, out := postSave(engine, "a.txt", "Bearer tok", []byte("x"))
	if rec.Code != http.StatusInternalServerError || out["success"] != false {
		t.Fatalf("got %d %v, want 500", rec.Code, out)
	}
}

func TestSuggestedFileNameIsNeverAPath(t *testing.T) {
	for in, want := range map[string]string{
		"export.jsonl":        "export.jsonl",
		"../../etc/passwd":    "passwd",
		`..\..\Windows\x.dll`: "x.dll",
		"/abs/dir/name.png":   "name.png",
		"":                    "download",
		"..":                  "download",
		"/":                   "download",
	} {
		if got := suggestedFileName(in); got != want {
			t.Errorf("suggestedFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSaveFileKeepsTheSuggestedExtension(t *testing.T) {
	dir := t.TempDir()
	engine := newSaveFileEngine(func(string) (string, error) { return filepath.Join(dir, "slices"), nil })
	rec, out := postSave(engine, "slices.zip", "Bearer tok", []byte("z"))
	if rec.Code != http.StatusOK || out["path"] != filepath.Join(dir, "slices.zip") {
		t.Fatalf("got %d %v, want the .zip extension added", rec.Code, out)
	}
	for in, want := range map[string]string{
		"/d/report.txt": "/d/report.txt", // the user's own extension wins
		"/d/report":     "/d/report.zip",
	} {
		if got := withSuggestedExtension(in, "x.zip"); got != want {
			t.Errorf("withSuggestedExtension(%q) = %q, want %q", in, got, want)
		}
	}
	if got := withSuggestedExtension("/d/report", "download"); got != "/d/report" {
		t.Errorf("no suggested extension: got %q", got)
	}
}
