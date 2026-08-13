package peer

import "errors"

// ErrNotFound is returned by Store lookups that miss. Sentinel so HTTP
// handlers can map it to 404 without string matching.
var ErrNotFound = errors.New("peer not found")

// ErrNameTaken is returned by Create/Save when the mention word is already
// in use — names are globally unique because @name must be unambiguous.
var ErrNameTaken = errors.New("peer name already taken")

// Store is the persistence seam. The SQLite-backed implementation lives in
// internal/db (dependency direction db → remote, same as ChatStoreInterface
// / session.SessionStore).
type Store interface {
	// Create persists a new peer. UUID/timestamps are assigned by the store
	// when empty. Fails with ErrNameTaken on a name collision.
	Create(p *Peer) error
	// Get returns one peer by uuid (ErrNotFound when missing).
	Get(uuid string) (Peer, error)
	// GetByToken returns the peer whose TokenHash matches the hash.
	GetByToken(tokenHash string) (Peer, error)
	// List returns all peers, newest first.
	List() ([]Peer, error)
	// ListByBot returns the peers bound to one bot.
	ListByBot(botUUID string) ([]Peer, error)
	// HasEnabledForBot reports whether the bot has ≥1 enabled peer — the
	// consumer's mount predicate ("a reason to run").
	HasEnabledForBot(botUUID string) bool
	// Save persists changed fields of an existing peer (ErrNotFound when
	// missing, ErrNameTaken on a name collision).
	Save(p *Peer) error
	// Delete removes the peer and its queued updates. Deleting a missing
	// peer is a no-op.
	Delete(uuid string) error

	// AppendUpdate persists one inbound update, assigns its ID, and enforces
	// MaxQueuedUpdates by dropping oldest updates beyond the cap. Returns
	// the number of dropped updates so the caller can log the truncation
	// (no silent caps).
	AppendUpdate(u *Update) (dropped int, err error)
	// UpdatesAfter returns up to limit updates with ID > afterID for the
	// peer, oldest first.
	UpdatesAfter(peerUUID string, afterID int64, limit int) ([]Update, error)
	// GetUpdate returns one update by id (ErrNotFound when missing or
	// already pruned). Used by send's reply threading.
	GetUpdate(peerUUID string, id int64) (Update, error)
	// AckUpdates advances the peer's cursor to upTo (never backwards) and
	// prunes confirmed updates.
	AckUpdates(peerUUID string, upTo int64) error
}
