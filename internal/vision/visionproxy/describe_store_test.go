package visionproxy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "tingly.db"), 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := conn.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return conn
}

func newTestStore(t *testing.T, conn *gorm.DB) *sqliteDescribeStore {
	t.Helper()
	store, err := NewSQLiteDescribeStore(conn)
	require.NoError(t, err)
	return store.(*sqliteDescribeStore)
}

func TestDescribeStore_RoundTrip(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	key := visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:abc"}

	_, ok := store.Get(key)
	require.False(t, ok, "empty store must miss")

	store.Put(key, "a red image")
	text, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, "a red image", text)

	// Upsert on the same key replaces, never duplicates.
	store.Put(key, "a redder image")
	text, ok = store.Get(key)
	require.True(t, ok)
	require.Equal(t, "a redder image", text)
	var count int64
	require.NoError(t, store.db.Model(&VisionDescriptionRecord{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestDescribeStore_IsolatesSessionAndService(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	base := visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:same"}
	store.Put(base, "for s1 on m1")

	other := base
	other.session = "user:s2"
	_, ok := store.Get(other)
	require.False(t, ok, "another session must not see s1's description")

	other = base
	other.model = "m2"
	_, ok = store.Get(other)
	require.False(t, ok, "another vision model must not see m1's description")
}

// TestDescribeCache_SurvivesRestart is the contract this tier exists for: a
// fresh cache (new process) over the same database answers from the store
// instead of missing — no vision call, byte-identical text.
func TestDescribeCache_SurvivesRestart(t *testing.T) {
	conn := openTestDB(t)
	key := visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:abc"}

	first := newDescribeCache(newTestStore(t, conn))
	first.put(key, "described once")

	// "Restart": a new cache, a new store handle, same database.
	second := newDescribeCache(newTestStore(t, conn))
	text, ok := second.get(key)
	require.True(t, ok, "must hit the store after restart")
	require.Equal(t, "described once", text)
}

// TestDescribeStore_NoAgeLimit pins the retention decision: a row is kept
// however old it is, as long as the table is under the size ceiling.
func TestDescribeStore_NoAgeLimit(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	old := visionCacheKey{session: "user:s1", content: "b64:old"}
	store.Put(old, "described a year ago")
	require.NoError(t, store.db.Model(&VisionDescriptionRecord{}).
		Where("content_hash = ?", old.content).
		Update("last_used_at", now.Add(-365*24*time.Hour)).Error)

	store.prune()
	text, ok := store.Get(old)
	require.True(t, ok, "age alone must never evict a row")
	require.Equal(t, "described a year ago", text)
}

func TestDescribeStore_PrunesToSizeCeiling(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	oldest := visionCacheKey{session: "user:s1", content: "b64:oldest"}
	store.Put(oldest, "x")
	require.NoError(t, store.db.Model(&VisionDescriptionRecord{}).
		Where("content_hash = ?", oldest.content).
		Update("last_used_at", now.Add(-time.Hour)).Error)

	// Fill past the ceiling; every bulk row was used more recently than
	// `oldest`, so `oldest` is the first to go and the table lands exactly
	// on the ceiling.
	var rows []VisionDescriptionRecord
	for i := 0; i < describeStoreMaxRows+5; i++ {
		rows = append(rows, VisionDescriptionRecord{
			Session: "user:bulk", ContentHash: "b64:" + time.Duration(i).String(),
			CreatedAt: now, LastUsedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, store.db.CreateInBatches(rows, 500).Error)
	store.prune()
	var count int64
	require.NoError(t, store.db.Model(&VisionDescriptionRecord{}).Count(&count).Error)
	require.EqualValues(t, describeStoreMaxRows, count)
	_, ok := store.Get(oldest)
	require.False(t, ok, "the least recently used row is the first to go")
}

func TestDescribeStore_GetTouchesLastUsed(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	key := visionCacheKey{session: "user:s1", content: "b64:1"}
	store.Put(key, "x")

	// Within the touch window: no write.
	now = now.Add(describeStoreTouchEvery / 2)
	_, _ = store.Get(key)
	var rec VisionDescriptionRecord
	require.NoError(t, store.db.First(&rec).Error)
	require.Equal(t, now.Add(-describeStoreTouchEvery/2), rec.LastUsedAt.UTC())

	// Past the window: last_used_at moves, keeping a live session's rows
	// at the back of the eviction order.
	now = now.Add(describeStoreTouchEvery)
	_, _ = store.Get(key)
	require.NoError(t, store.db.First(&rec).Error)
	require.Equal(t, now, rec.LastUsedAt.UTC())
}

// TestVisionProxy_Cache_RestartDoesNotRedescribe drives the whole processor:
// a session's image is described once; a new processor (new process) over
// the same database splices the stored text with zero vision calls.
func TestVisionProxy_Cache_RestartDoesNotRedescribe(t *testing.T) {
	conn := openTestDB(t)
	prov := mkProvider("anthropic-vision")
	svcs := []*loadbalance.Service{mkService(prov.UUID, true)}
	session := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-a"}

	fake1 := newFakeVisionClient("a V-tail aircraft")
	p1 := mkProcessor(t, fake1, prov)
	p1.cache = newDescribeCache(newTestStore(t, conn))
	req1 := betaReqWithImages("what is this?", tinyPNGBase64)
	require.NoError(t, p1.Process(context.Background(), req1, svcs, session))
	require.Equal(t, 1, fake1.callCount())

	fake2 := newFakeVisionClient("a totally different wording")
	p2 := mkProcessor(t, fake2, prov)
	p2.cache = newDescribeCache(newTestStore(t, conn))
	req2 := betaReqWithImages("what is this?", tinyPNGBase64)
	require.NoError(t, p2.Process(context.Background(), req2, svcs, session))
	require.Equal(t, 0, fake2.callCount(), "after restart the image must come from the store")
	require.Equal(t, req1.Messages[0].Content[1].OfText.Text, req2.Messages[0].Content[1].OfText.Text,
		"the replacement text must be byte-identical across the restart")
}

// TestDescribeStore_BootPruneArmsThrottle: the boot-time prune counts as
// the first prune, so the first Put of the process does not run it again.
func TestDescribeStore_BootPruneArmsThrottle(t *testing.T) {
	store := newTestStore(t, openTestDB(t))
	require.False(t, store.lastPrune.IsZero(), "constructor must stamp lastPrune")
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	store.lastPrune = now
	store.Put(visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:1"}, "x")
	require.Equal(t, now, store.lastPrune, "a Put within the window must not re-prune")
	now = now.Add(describeStorePruneEvery)
	store.Put(visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:2"}, "y")
	require.Equal(t, now, store.lastPrune, "a Put past the window prunes and re-arms")
}

// TestDescribeCache_EmptyServiceKeySkipsStore: nothing is ever written
// under an empty service key, so a lookup must not cost a SELECT.
func TestDescribeCache_EmptyServiceKeySkipsStore(t *testing.T) {
	store := &countingStore{}
	c := newDescribeCache(store)
	_, ok := c.get(visionCacheKey{session: "user:s1", content: "b64:1"})
	require.False(t, ok)
	require.Equal(t, 0, store.gets, "no service → no store round trip")
	_, _ = c.get(visionCacheKey{session: "user:s1", provider: "p1", model: "m1", content: "b64:1"})
	require.Equal(t, 1, store.gets)
}

type countingStore struct{ gets int }

func (s *countingStore) Get(visionCacheKey) (string, bool) { s.gets++; return "", false }
func (s *countingStore) Put(visionCacheKey, string)        {}

func TestSessionScope_IgnoresIPBackup(t *testing.T) {
	home := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-a", IPBackup: "10.0.0.2"}
	office := typ.SessionID{Source: typ.SessionSourceUser, Value: "session-a", IPBackup: "192.168.1.9"}
	require.Equal(t, sessionScope(home), sessionScope(office),
		"a network change must not invalidate a conversation's descriptions")
	require.NotEqual(t, sessionScope(home),
		sessionScope(typ.SessionID{Source: typ.SessionSourceHeader, Value: "session-a"}),
		"the same value from a different source is a different session")
}
