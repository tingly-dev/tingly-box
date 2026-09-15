package managedagent

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

// recoveryListLimit bounds the startup scan; anything beyond it is not a
// realistic single-host session count.
const recoveryListLimit = 100000

// RecoverOnStart reconciles sessions that were live when the previous
// process died. Runs (and their Claude Code processes) do not survive a
// restart, so:
//
//   - running / waiting_input → idle, with a status event saying why. The
//     Claude Code session id is kept, so the next message resumes it.
//   - queued → started again through the Launcher (the prompt is already in
//     the log).
//
// Nothing on disk is reconciled: the agent works in the user's own folders,
// so there is never a checkout of ours to clean up.
func (s *Service) RecoverOnStart(ctx context.Context) error {
	// Every active session, not the list page's default cap.
	active, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{Active: true, Limit: recoveryListLimit})
	if err != nil {
		return err
	}
	var errs []error
	for i := range active {
		sess := &active[i]
		switch sess.Status {
		case SessionRunning, SessionWaitingInput:
			now := s.now()
			sess.Status, sess.LastActiveAt = SessionIdle, now
			if err := s.stores.Sessions.UpdateSession(ctx, sess); err != nil {
				errs = append(errs, err)
				continue
			}
			_ = s.stores.Events.AppendEvent(ctx, &Event{SessionID: sess.ID, Kind: EventStatus,
				Text: string(SessionIdle) + ": interrupted by restart; send a message to resume", At: now})
		case SessionQueued:
			if s.launcher == nil {
				continue
			}
			folder, err := s.stores.Folders.GetFolder(ctx, sess.FolderID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if err := s.launcher.Start(ctx, Run{Session: sess, Folder: folder}); err != nil {
				sess.Status, sess.Error = SessionFailed, "restart: "+err.Error()
				_ = s.stores.Sessions.UpdateSession(ctx, sess)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// Shutdown stops every live run so no agent process outlives the server.
// Sessions are left as they are; RecoverOnStart reconciles them on the
// next start (running → idle, resumable).
func (s *Service) Shutdown(ctx context.Context) {
	if s.launcher == nil {
		return
	}
	active, err := s.stores.Sessions.ListSessions(ctx, SessionFilter{Active: true, Limit: recoveryListLimit})
	if err != nil {
		logrus.WithError(err).Warn("managed agent: shutdown could not list sessions")
		return
	}
	for i := range active {
		if active[i].Status == SessionRunning || active[i].Status == SessionWaitingInput {
			_ = s.launcher.Stop(ctx, active[i].ID)
		}
	}
}
