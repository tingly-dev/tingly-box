package imagestore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// A workbench is a focus: one root image and a description of the subject in
// it, plus the images derived from it. It does not own image files — it
// references archive ids — so deleting a workbench never deletes an image,
// and the same image can sit in two workbenches.
//
// Stored as one JSON file per workbench under <imageDir>/workbench/, next to
// the images it references, so the folder stays self-contained.

// ErrRootInUse is returned when deleting an image that is some workbench's root.
var ErrRootInUse = errors.New("image is the root of a workbench")

const workbenchDirName = "workbench"

var workbenchIDPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

// WorkbenchItem is one image derived inside a workbench. ParentID is the image
// it was made from: the root, or another item.
type WorkbenchItem struct {
	ImageID  string    `json:"image_id"`
	ParentID string    `json:"parent_id"`
	AddedAt  time.Time `json:"added_at"`
}

// Workbench is the persisted shape.
type Workbench struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	RootImageID string          `json:"root_image_id"`
	Items       []WorkbenchItem `json:"items"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Workbenches manages the workbench files of one Store.
type Workbenches struct {
	images *Store
	mu     sync.Mutex
}

// NewWorkbenches returns the workbench manager for images' directory.
func NewWorkbenches(images *Store) *Workbenches {
	return &Workbenches{images: images}
}

func (w *Workbenches) dir() string { return filepath.Join(w.images.root, workbenchDirName) }

func (w *Workbenches) path(id string) (string, error) {
	if !workbenchIDPattern.MatchString(id) {
		return "", ErrInvalidID
	}
	return filepath.Join(w.dir(), id+".json"), nil
}

func (w *Workbenches) read(id string) (*Workbench, error) {
	p, err := w.path(id)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var wb Workbench
	if err := json.Unmarshal(raw, &wb); err != nil {
		return nil, fmt.Errorf("read workbench %s: %w", id, err)
	}
	if wb.Items == nil {
		wb.Items = []WorkbenchItem{}
	}
	return &wb, nil
}

// write replaces the file atomically, so a reader never sees half of it.
func (w *Workbenches) write(wb *Workbench) error {
	p, err := w.path(wb.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(w.dir(), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(wb, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(w.dir(), wb.ID+".*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// List returns every workbench, most recently updated first. A file that
// cannot be parsed is skipped rather than failing the whole list.
func (w *Workbenches) List() ([]Workbench, error) {
	entries, err := os.ReadDir(w.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return []Workbench{}, nil
		}
		return nil, err
	}
	out := []Workbench{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if e.IsDir() || !ok || !workbenchIDPattern.MatchString(id) {
			continue
		}
		wb, err := w.read(id)
		if err != nil {
			continue
		}
		out = append(out, *wb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

// Get returns one workbench.
func (w *Workbenches) Get(id string) (*Workbench, error) {
	return w.read(id)
}

// Create starts a workbench on an archived image.
func (w *Workbenches) Create(name, description, rootImageID string) (*Workbench, error) {
	if !w.images.Exists(rootImageID) {
		return nil, fmt.Errorf("root image %q: %w", rootImageID, ErrNotFound)
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	now := w.images.now()
	wb := &Workbench{
		ID:          hex.EncodeToString(b[:]),
		Name:        strings.TrimSpace(name),
		Description: description,
		RootImageID: rootImageID,
		Items:       []WorkbenchItem{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.write(wb); err != nil {
		return nil, err
	}
	return wb, nil
}

// Update changes the name and/or description; nil leaves a field as is.
func (w *Workbenches) Update(id string, name, description *string) (*Workbench, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	wb, err := w.read(id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		wb.Name = strings.TrimSpace(*name)
	}
	if description != nil {
		wb.Description = *description
	}
	wb.UpdatedAt = w.images.now()
	if err := w.write(wb); err != nil {
		return nil, err
	}
	return wb, nil
}

// Delete removes the workbench. Its images stay in the archive.
func (w *Workbenches) Delete(id string) error {
	p, err := w.path(id)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// AddItems records images derived inside the workbench. Every image must be
// archived. A parent that is neither the root nor an item of this workbench
// (or is empty) is recorded as the root: the image was made while focused on
// this subject, so the root is the honest default. An image already present
// is left where it is.
func (w *Workbenches) AddItems(id string, items []WorkbenchItem) (*Workbench, error) {
	for _, it := range items {
		if !w.images.Exists(it.ImageID) {
			return nil, fmt.Errorf("image %q: %w", it.ImageID, ErrNotFound)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	wb, err := w.read(id)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{wb.RootImageID: true}
	for _, it := range wb.Items {
		known[it.ImageID] = true
	}
	now := w.images.now()
	for _, it := range items {
		if known[it.ImageID] {
			continue
		}
		parent := it.ParentID
		if !known[parent] {
			parent = wb.RootImageID
		}
		wb.Items = append(wb.Items, WorkbenchItem{ImageID: it.ImageID, ParentID: parent, AddedAt: now})
		known[it.ImageID] = true
	}
	wb.UpdatedAt = now
	if err := w.write(wb); err != nil {
		return nil, err
	}
	return wb, nil
}

// RemoveItem takes an image out of the workbench (the file stays archived).
// Its children are re-parented to its own parent, so the lineage stays a tree.
func (w *Workbenches) RemoveItem(id, imageID string) (*Workbench, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	wb, err := w.read(id)
	if err != nil {
		return nil, err
	}
	idx := -1
	for i, it := range wb.Items {
		if it.ImageID == imageID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, ErrNotFound
	}
	parent := wb.Items[idx].ParentID
	wb.Items = append(wb.Items[:idx], wb.Items[idx+1:]...)
	for i := range wb.Items {
		if wb.Items[i].ParentID == imageID {
			wb.Items[i].ParentID = parent
		}
	}
	wb.UpdatedAt = w.images.now()
	if err := w.write(wb); err != nil {
		return nil, err
	}
	return wb, nil
}

// RootOf returns the workbenches whose root is imageID.
func (w *Workbenches) RootOf(imageID string) []Workbench {
	all, _ := w.List()
	var out []Workbench
	for _, wb := range all {
		if wb.RootImageID == imageID {
			out = append(out, wb)
		}
	}
	return out
}

// DeleteImage deletes an archived image, refusing when it is a workbench
// root (the workbench would be left focused on nothing), and dropping it from
// every workbench it is an item of.
func (w *Workbenches) DeleteImage(imageID string) error {
	if _, err := w.images.Path(imageID); err != nil {
		return err
	}
	if roots := w.RootOf(imageID); len(roots) > 0 {
		return fmt.Errorf("%w %q", ErrRootInUse, roots[0].Name)
	}
	all, _ := w.List()
	for _, wb := range all {
		for _, it := range wb.Items {
			if it.ImageID == imageID {
				if _, err := w.RemoveItem(wb.ID, imageID); err != nil {
					return err
				}
				break
			}
		}
	}
	return w.images.Delete(imageID)
}
