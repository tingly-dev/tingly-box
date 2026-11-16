package bot_test

import (
	"testing"

	"github.com/tingly-dev/tingly-box/remote/bot"
)

// openStore returns an in-memory chat store. dir is ignored; persistence
// across reopening is the host database's concern and is tested there
// (internal/db).
func openStore(t *testing.T, dir string) bot.ChatStoreInterface {
	t.Helper()
	return bot.NewMemoryChatStore()
}
