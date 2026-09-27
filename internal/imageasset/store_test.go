package imageasset

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(t.TempDir())
	clock := time.UnixMilli(1_700_000_000_000)
	store.now = func() time.Time {
		clock = clock.Add(time.Second)
		return clock
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func pngBytes(t *testing.T, width, height int, shade uint8) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: shade, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNewStoreTouchesNothingUntilUsed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "image-assets")
	store := NewStore(dir)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("store created %s before first use", dir)
	}
}

func TestSavePiecesCreatesAndNormalizes(t *testing.T) {
	store := newTestStore(t)
	saved, err := store.SavePieces([]PromptPieceInput{
		{Kind: KindPrompt, Title: " Rainy street ", Text: " a quiet street after rain ", Tags: []string{"Street", "street", " "}},
		{Kind: KindTerm, Title: "ignored", Text: "rim lighting"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved[0].ID == "" {
		t.Fatalf("saved = %+v", saved)
	}
	if saved[0].Title != "Rainy street" || saved[0].Text != "a quiet street after rain" {
		t.Errorf("not trimmed: %+v", saved[0])
	}
	if len(saved[0].Tags) != 1 || saved[0].Tags[0] != "street" {
		t.Errorf("tags = %v", saved[0].Tags)
	}
	if saved[1].Title != "" {
		t.Errorf("a term kept a title: %q", saved[1].Title)
	}

	listed, err := store.ListPieces()
	if err != nil {
		t.Fatal(err)
	}
	// Most recently edited first; a batch is ordered, so the term is newer.
	if len(listed) != 2 || listed[0].Text != "rim lighting" {
		t.Fatalf("listed = %+v", listed)
	}
}

func TestSavePiecesUpdateKeepsCreatedAt(t *testing.T) {
	store := newTestStore(t)
	created, err := store.SavePieces([]PromptPieceInput{{Kind: KindPrompt, Text: "fox"}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.SavePieces([]PromptPieceInput{{ID: created[0].ID, Kind: KindPrompt, Title: "Fox", Text: "a fox in snow"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated[0].CreatedAt != created[0].CreatedAt || updated[0].UpdatedAt <= created[0].UpdatedAt {
		t.Fatalf("created %+v, updated %+v", created[0], updated[0])
	}
	if listed, _ := store.ListPieces(); len(listed) != 1 || listed[0].Text != "a fox in snow" {
		t.Fatalf("listed = %+v", listed)
	}
}

func TestSavePiecesIsAllOrNothing(t *testing.T) {
	store := newTestStore(t)
	_, err := store.SavePieces([]PromptPieceInput{
		{Kind: KindTerm, Text: "kept?"},
		{ID: "missing", Kind: KindTerm, Text: "update of nothing"},
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if listed, _ := store.ListPieces(); len(listed) != 0 {
		t.Fatalf("a failed batch left %d pieces", len(listed))
	}
}

func TestSavePiecesRejectsInvalid(t *testing.T) {
	store := newTestStore(t)
	for name, in := range map[string]PromptPieceInput{
		"kind":  {Kind: "sentence", Text: "x"},
		"empty": {Kind: KindTerm, Text: "   "},
	} {
		if _, err := store.SavePieces([]PromptPieceInput{in}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDeletePiece(t *testing.T) {
	store := newTestStore(t)
	saved, _ := store.SavePieces([]PromptPieceInput{{Kind: KindTerm, Text: "35mm"}})
	if err := store.DeletePiece(saved[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePiece(saved[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v", err)
	}
}

func TestAddReferencesStoresFileAndDeduplicates(t *testing.T) {
	store := newTestStore(t)
	data := pngBytes(t, 3, 2, 10)
	added, err := store.AddReferences([]ReferenceInput{
		{Name: "sheet.png", Data: data},
		{Name: "again.png", Data: data},
		{Name: "other.png", Data: pngBytes(t, 4, 4, 20)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 3 || added[0].Existing || !added[1].Existing || added[1].ID != added[0].ID || added[2].Existing {
		t.Fatalf("added = %+v", added)
	}
	first := added[0].ReferenceImage
	if first.MIME != "image/png" || first.Width != 3 || first.Height != 2 || first.Bytes != int64(len(data)) {
		t.Errorf("metadata = %+v", first)
	}

	path, mime, err := store.ReferenceFile(first.ID)
	if err != nil || mime != "image/png" {
		t.Fatalf("ReferenceFile = %q, %q, %v", path, mime, err)
	}
	if onDisk, _ := os.ReadFile(path); !bytes.Equal(onDisk, data) {
		t.Fatal("file content differs from what was added")
	}

	// A later request with the same bytes comes back as the existing record.
	again, err := store.AddReferences([]ReferenceInput{{Name: "third.png", Data: data}})
	if err != nil || !again[0].Existing || again[0].ID != first.ID {
		t.Fatalf("again = %+v, %v", again, err)
	}
	if listed, _ := store.ListReferences(); len(listed) != 2 {
		t.Fatalf("listed %d references, want 2", len(listed))
	}
}

func TestAddReferencesRejectsNonImagesBeforeWriting(t *testing.T) {
	store := newTestStore(t)
	_, err := store.AddReferences([]ReferenceInput{
		{Name: "ok.png", Data: pngBytes(t, 1, 1, 0)},
		{Name: "notes.txt", Data: []byte("not an image")},
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if listed, _ := store.ListReferences(); len(listed) != 0 {
		t.Fatalf("a rejected request kept %d images", len(listed))
	}
}

func TestRenameAndDeleteReference(t *testing.T) {
	store := newTestStore(t)
	added, _ := store.AddReferences([]ReferenceInput{{Name: "a.png", Data: pngBytes(t, 1, 1, 0)}})
	id := added[0].ID
	renamed, err := store.RenameReference(id, " hero.png ")
	if err != nil || renamed.Name != "hero.png" {
		t.Fatalf("renamed = %+v, %v", renamed, err)
	}
	path, _, _ := store.ReferenceFile(id)
	if err := store.DeleteReference(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file left behind after delete")
	}
	if _, _, err := store.ReferenceFile(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWebPSize(t *testing.T) {
	// A minimal VP8L header for a 5×7 image: width-1 and height-1 packed into
	// 14-bit fields after the 0x2f signature.
	bits := uint32(4) | uint32(6)<<14
	chunk := []byte{'V', 'P', '8', 'L', 0, 0, 0, 0, 0x2f, byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24)}
	data := append([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}, chunk...)
	data = append(data, make([]byte, 20)...)
	if w, h := webpSize(data); w != 5 || h != 7 {
		t.Fatalf("webpSize = %d×%d, want 5×7", w, h)
	}
}
