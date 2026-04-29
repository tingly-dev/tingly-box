package obs

import (
	"container/list"
	"os"
)

// fileLRU is a bounded cache of open *os.File handles keyed by path.
// The least-recently-used entry is evicted (with fsync+close) when the
// capacity is exceeded. Not goroutine-safe; call only from a single goroutine.
type fileLRU struct {
	cap   int
	index map[string]*list.Element
	order *list.List // front = most recent
}

type lruEntry struct {
	path string
	file *os.File
}

func newFileLRU(cap int) *fileLRU {
	if cap <= 0 {
		cap = 256
	}
	return &fileLRU{
		cap:   cap,
		index: make(map[string]*list.Element),
		order: list.New(),
	}
}

// get returns the file for path, or nil if not cached.
func (l *fileLRU) get(path string) *os.File {
	elem, ok := l.index[path]
	if !ok {
		return nil
	}
	l.order.MoveToFront(elem)
	return elem.Value.(*lruEntry).file
}

// put adds or replaces the file for path, evicting LRU entries as needed.
// The caller must not close f after passing it here.
func (l *fileLRU) put(path string, f *os.File) {
	if elem, ok := l.index[path]; ok {
		l.order.MoveToFront(elem)
		old := elem.Value.(*lruEntry)
		if old.file != f {
			syncAndClose(old.file)
			old.file = f
		}
		return
	}
	for l.order.Len() >= l.cap {
		l.evictLRU()
	}
	entry := &lruEntry{path: path, file: f}
	elem := l.order.PushFront(entry)
	l.index[path] = elem
}

// closeAll syncs and closes every open file.
func (l *fileLRU) closeAll() {
	for l.order.Len() > 0 {
		l.evictLRU()
	}
}

func (l *fileLRU) evictLRU() {
	back := l.order.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*lruEntry)
	syncAndClose(entry.file)
	delete(l.index, entry.path)
	l.order.Remove(back)
}

func syncAndClose(f *os.File) {
	if f != nil {
		_ = f.Sync()
		_ = f.Close()
	}
}
