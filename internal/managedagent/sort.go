package managedagent

import (
	"sort"

	"github.com/tingly-dev/tingly-box/remote/session"
)

func sortSessionsByActivity(sessions []session.Session) {
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].LastActivity.After(sessions[j].LastActivity) })
}

func sortRecentFolders(folders []RecentFolder) {
	sort.Slice(folders, func(i, j int) bool { return folders[i].LastUsedAt.After(folders[j].LastUsedAt) })
}
