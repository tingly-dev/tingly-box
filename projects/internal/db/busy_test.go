package db

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A transaction that reads and then writes must wait for a competing writer
// instead of failing with "database is locked". SQLite never retries a
// deferred transaction that has to upgrade from reader to writer after another
// writer committed, whatever the busy timeout; transactions therefore begin
// as writers (_txlock=immediate in Open), and concurrent chats, threads and
// the web can write at the same time.
func TestConcurrentReadThenWriteTransactionsWait(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "projects.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	const writers, rounds = 8, 40
	errs := make(chan error, writers*rounds)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				errs <- s.db.Transaction(func(tx *gorm.DB) error {
					var n int64
					if err := tx.Model(&Project{}).Count(&n).Error; err != nil {
						return err
					}
					return tx.Create(&Project{ID: fmt.Sprintf("p-%d-%d", w, i), Name: "n"}).Error
				})
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}
