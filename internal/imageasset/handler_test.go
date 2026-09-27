package imageasset

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/swagger"
)

func newTestServer(t *testing.T) (*gin.Engine, *Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	store := newTestStore(t)
	manager := swagger.NewRouteManager(engine)
	RegisterRoutes(manager.NewGroup("api", "v1", ""), NewHandler(store))
	return engine, store
}

func call(t *testing.T, engine *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestPiecesAPI(t *testing.T) {
	engine, _ := newTestServer(t)

	rec := call(t, engine, http.MethodPost, "/api/v1/image-assets/pieces", ImageAssetSavePiecesRequest{
		Pieces: []PromptPieceInput{{Kind: KindPrompt, Text: "a fox", Tags: []string{"Animal"}}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	var saved ImageAssetPiecesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &saved)
	if !saved.Success || len(saved.Pieces) != 1 || saved.Pieces[0].Tags[0] != "animal" {
		t.Fatalf("saved = %+v", saved)
	}

	rec = call(t, engine, http.MethodGet, "/api/v1/image-assets/pieces", nil)
	var listed ImageAssetPiecesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &listed)
	if rec.Code != http.StatusOK || len(listed.Pieces) != 1 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	if rec = call(t, engine, http.MethodPost, "/api/v1/image-assets/pieces", ImageAssetSavePiecesRequest{
		Pieces: []PromptPieceInput{{Kind: "sentence", Text: "x"}},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind: %d", rec.Code)
	}

	if rec = call(t, engine, http.MethodDelete, "/api/v1/image-assets/pieces/"+saved.Pieces[0].ID, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec = call(t, engine, http.MethodDelete, "/api/v1/image-assets/pieces/"+saved.Pieces[0].ID, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", rec.Code)
	}
}

func TestReferencesAPI(t *testing.T) {
	engine, _ := newTestServer(t)
	data := pngBytes(t, 2, 2, 5)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)

	rec := call(t, engine, http.MethodPost, "/api/v1/image-assets/references", ImageAssetAddReferencesRequest{
		References: []ImageAssetReferenceUpload{{Name: "sheet.png", DataURL: dataURL}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	var added ImageAssetAddReferencesResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &added)
	id := added.References[0].ID

	rec = call(t, engine, http.MethodGet, "/api/v1/image-assets/references/"+id+"/content", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("content: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = call(t, engine, http.MethodPut, "/api/v1/image-assets/references/"+id, ImageAssetRenameReferenceRequest{Name: "hero.png"})
	var renamed ImageAssetReferenceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &renamed)
	if rec.Code != http.StatusOK || renamed.Reference.Name != "hero.png" {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}

	if rec = call(t, engine, http.MethodPost, "/api/v1/image-assets/references", ImageAssetAddReferencesRequest{
		References: []ImageAssetReferenceUpload{{Name: "x.png", DataURL: "not a data url"}},
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad data url: %d", rec.Code)
	}

	if rec = call(t, engine, http.MethodDelete, "/api/v1/image-assets/references/"+id, nil); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec = call(t, engine, http.MethodGet, "/api/v1/image-assets/references/"+id+"/content", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("content after delete: %d", rec.Code)
	}
}
