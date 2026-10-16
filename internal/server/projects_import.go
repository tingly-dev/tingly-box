package server

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/internal/db"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// deskChatID is the ChatID the retired Desk kept its sessions under in the
// remote session store (shared with IM @cc sessions, which stay there).
const deskChatID = "web"

// importDeskSessions brings the retired Desk's sessions into Projects as
// working-folder sessions, once (Projects remembers the "desk" source). The
// originals are left in the remote session store untouched. Each keeps its
// Claude Code session id, so the next message resumes the conversation.
func importDeskSessions(sm *db.StoreManager, app *projectsapp.App) {
	if sm == nil || sm.RemoteSessions() == nil {
		return
	}
	store := sm.RemoteSessions()
	list, err := store.ListByChat(deskChatID)
	if err != nil {
		logrus.WithError(err).Warn("Projects: could not list Desk sessions to import")
		return
	}
	in := make([]projectsapp.ImportedSession, 0, len(list))
	for _, s := range list {
		msgs, err := store.Messages(s.ID)
		if err != nil {
			logrus.WithError(err).WithField("session", s.ID).Warn("Projects: could not read a Desk session's transcript")
		}
		in = append(in, deskSession(s, msgs))
	}
	n, err := app.ImportSessions("desk", in)
	if err != nil {
		logrus.WithError(err).Warn("Projects: some Desk sessions were not imported; retrying next start")
	}
	if n > 0 {
		logrus.WithField("count", n).Info("Projects: imported Desk sessions as working-folder sessions")
	}
}

func deskSession(s *session.Session, msgs []session.Message) projectsapp.ImportedSession {
	entries := make([]engine.Entry, 0, len(msgs))
	for _, m := range msgs {
		entries = append(entries, engine.Entry{
			Role: m.Role, Content: m.Content, Kind: m.Kind, RequestID: m.RequestID,
			Payload: m.Payload, Parent: m.Parent, Timestamp: m.Timestamp,
		})
	}
	return projectsapp.ImportedSession{
		SessionID: s.ID, Folder: s.Project, Task: s.Request,
		Closed:         s.Status == session.StatusClosed || s.Status == session.StatusExpired,
		PermissionMode: s.PermissionMode, Profile: s.Profile, Model: s.Model,
		CreatedAt: s.CreatedAt, UpdatedAt: s.LastActivity, Entries: entries,
	}
}
