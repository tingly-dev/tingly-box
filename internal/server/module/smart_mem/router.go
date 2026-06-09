package smart_mem

// Router resolves a caller-supplied key (today: a UUID) to the persisted
// document bytes. The interface exists so smarter routing strategies
// (prefix namespaces, multi-store fan-out, LLM-assisted lookup) can be
// dropped in without changing the HTTP handler.
type Router interface {
	Resolve(key string) ([]byte, error)
}

// UUIDRouter is the default Router: it treats the key as a UUID and
// reads directly from the backing FileStore.
type UUIDRouter struct {
	store *FileStore
}

// NewUUIDRouter wires a UUIDRouter to the given FileStore.
func NewUUIDRouter(store *FileStore) *UUIDRouter {
	return &UUIDRouter{store: store}
}

// Resolve returns the document bytes for the UUID key.
func (r *UUIDRouter) Resolve(key string) ([]byte, error) {
	return r.store.Get(key)
}
