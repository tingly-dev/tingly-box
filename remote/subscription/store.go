package subscription

import "errors"

// ErrNotFound is returned by Store lookups that miss. Sentinel so HTTP
// handlers can map it to 404 without string matching.
var ErrNotFound = errors.New("subscription not found")

// ErrNameTaken is returned by Create/Update when the mention word is already
// in use — names are globally unique because @name must be unambiguous.
var ErrNameTaken = errors.New("subscription name already taken")

// Store is the persistence seam. The SQLite-backed implementation lives in
// internal/data/db (dependency direction db → remote, same as
// ChatStoreInterface / session.SessionStore).
type Store interface {
	// Create persists a new subscription. UUID/timestamps are assigned by
	// the store when empty. Fails with ErrNameTaken on a name collision.
	Create(sub *Subscription) error
	// Get returns one subscription by uuid (ErrNotFound when missing).
	Get(uuid string) (Subscription, error)
	// GetByToken returns the subscription whose TokenHash matches the hash.
	GetByToken(tokenHash string) (Subscription, error)
	// List returns all subscriptions, newest first.
	List() ([]Subscription, error)
	// ListByBot returns the subscriptions bound to one bot.
	ListByBot(botUUID string) ([]Subscription, error)
	// HasEnabledForBot reports whether the bot has ≥1 enabled subscription —
	// the consumer's mount predicate ("a reason to run").
	HasEnabledForBot(botUUID string) bool
	// Update persists changed fields of an existing subscription
	// (ErrNotFound when missing, ErrNameTaken on a name collision).
	Update(sub *Subscription) error
	// Delete removes the subscription and its queued events. Deleting a
	// missing subscription is a no-op.
	Delete(uuid string) error

	// AppendEvent persists one inbound event, assigns its ID, and enforces
	// MaxQueuedEvents by dropping oldest events beyond the cap. Returns the
	// number of dropped events so the caller can log the truncation
	// (no silent caps).
	AppendEvent(ev *Event) (dropped int, err error)
	// EventsAfter returns up to limit events with ID > afterID for the
	// subscription, oldest first.
	EventsAfter(subscriptionUUID string, afterID int64, limit int) ([]Event, error)
	// GetEvent returns one event by id (ErrNotFound when missing or already
	// pruned). Used by reply threading.
	GetEvent(subscriptionUUID string, id int64) (Event, error)
	// AckEvents advances the subscription's cursor to upTo (never backwards)
	// and prunes acked events.
	AckEvents(subscriptionUUID string, upTo int64) error
}
