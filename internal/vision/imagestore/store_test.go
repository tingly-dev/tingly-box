package imagestore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedStore(t *testing.T, at time.Time) *Store {
	t.Helper()
	s := New(t.TempDir())
	s.now = func() time.Time { return at }
	return s
}

func TestSaveSameSecondDoesNotOverwrite(t *testing.T) {
	// The predecessor named files by the second, so two responses landing in
	// the same second overwrote each other.
	s := fixedStore(t, time.Date(2026, 9, 30, 15, 30, 12, 0, time.Local))
	a, err := s.Save([]byte("a"), Meta{Prompt: "first"})
	require.NoError(t, err)
	b, err := s.Save([]byte("b"), Meta{Prompt: "second"})
	require.NoError(t, err)
	require.NotEqual(t, a, b)

	imgs, err := s.List(0, "")
	require.NoError(t, err)
	assert.Len(t, imgs, 2)
	got, err := s.Get(a)
	require.NoError(t, err)
	assert.Equal(t, "first", got.Meta.Prompt)
}

func TestListNewestFirstAndPaging(t *testing.T) {
	s := New(t.TempDir())
	var ids []string
	for i := 0; i < 5; i++ {
		at := time.Date(2026, 9, 28+i/2, 10, i, 0, 0, time.Local)
		s.now = func() time.Time { return at }
		id, err := s.Save([]byte{byte(i)}, Meta{Prompt: "p"})
		require.NoError(t, err)
		ids = append(ids, id)
	}
	page1, err := s.List(2, "")
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, ids[4], page1[0].ID)
	assert.Equal(t, ids[3], page1[1].ID)

	page2, err := s.List(10, page1[1].ID)
	require.NoError(t, err)
	require.Len(t, page2, 3)
	assert.Equal(t, []string{ids[2], ids[1], ids[0]}, []string{page2[0].ID, page2[1].ID, page2[2].ID})
}

func TestListReadsLegacyFilesAndSkipsStrangers(t *testing.T) {
	root := t.TempDir()
	day := filepath.Join(root, "20260901")
	require.NoError(t, os.MkdirAll(day, 0700))
	// Written by the predecessor: second-resolution name, -N for the Nth image.
	require.NoError(t, os.WriteFile(filepath.Join(day, "20260901-101010.png"), []byte("x"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(day, "20260901-101010-1.png"), []byte("y"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(day, "20260901-101010.txt"),
		[]byte("Prompt: a cat\n\nwith a hat\n\nModel: m\nSize: 1024x1024\nQuality: high\nTimestamp: x\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(day, "holiday.png"), []byte("z"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "workbench"), 0700))

	imgs, err := New(root).List(0, "")
	require.NoError(t, err)
	require.Len(t, imgs, 2)
	got, err := New(root).Get("20260901-101010")
	require.NoError(t, err)
	assert.Equal(t, "a cat\n\nwith a hat", got.Meta.Prompt)
	assert.Equal(t, "m", got.Meta.Model)
	assert.Equal(t, "high", got.Meta.Quality)
}

func TestPathRejectsTraversal(t *testing.T) {
	s := New(t.TempDir())
	for _, id := range []string{"../x", "20260901-101010/../../etc", "", "20260901-101010.png"} {
		_, err := s.Path(id)
		assert.True(t, errors.Is(err, ErrInvalidID), id)
	}
}

func TestMetaRoundTrip(t *testing.T) {
	in := Meta{Prompt: "line one\n\nModel: not really", Operation: "edit", Model: "gpt-image-2", Size: "1024x1024", Quality: "high"}
	out := ParseMeta(FormatMeta(in, time.Now()))
	assert.Equal(t, in, out)
}

func TestWorkbenchLifecycle(t *testing.T) {
	s := New(t.TempDir())
	root, err := s.Save([]byte("root"), Meta{})
	require.NoError(t, err)
	child, err := s.Save([]byte("child"), Meta{})
	require.NoError(t, err)
	grandchild, err := s.Save([]byte("grandchild"), Meta{})
	require.NoError(t, err)
	w := NewWorkbenches(s)

	_, err = w.Create("nope", "", "20260101-000000-abcdef")
	assert.ErrorIs(t, err, ErrNotFound)

	wb, err := w.Create(" Mira ", "red scarf, short hair", root)
	require.NoError(t, err)
	assert.Equal(t, "Mira", wb.Name)

	wb, err = w.AddItems(wb.ID, []WorkbenchItem{
		{ImageID: child},
		{ImageID: grandchild, ParentID: child},
		{ImageID: child, ParentID: grandchild}, // duplicate: stays where it is
	})
	require.NoError(t, err)
	require.Len(t, wb.Items, 2)
	assert.Equal(t, root, wb.Items[0].ParentID, "no parent → the root")
	assert.Equal(t, child, wb.Items[1].ParentID)

	// The root cannot be deleted out from under its workbench.
	assert.ErrorIs(t, w.DeleteImage(root), ErrRootInUse)

	// Deleting an item's image re-parents its children and drops it.
	require.NoError(t, w.DeleteImage(child))
	wb, err = w.Get(wb.ID)
	require.NoError(t, err)
	require.Len(t, wb.Items, 1)
	assert.Equal(t, grandchild, wb.Items[0].ImageID)
	assert.Equal(t, root, wb.Items[0].ParentID)

	desc := "blue scarf"
	wb, err = w.Update(wb.ID, nil, &desc)
	require.NoError(t, err)
	assert.Equal(t, "Mira", wb.Name)
	assert.Equal(t, "blue scarf", wb.Description)

	list, err := w.List()
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, w.Delete(wb.ID))
	assert.True(t, s.Exists(root), "deleting a workbench keeps its images")
	assert.True(t, s.Exists(grandchild))
	_, err = w.Get(wb.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}
