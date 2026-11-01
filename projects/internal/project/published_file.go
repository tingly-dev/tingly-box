package project

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// PublishFile copies a thread's output to the project library and publishes a
// persistent conversation event. The source must be inside the thread's work
// directory or project library, including after resolving symbolic links.
func (s *Service) PublishFile(ctx context.Context, threadID, source, caption string) (*db.ThreadEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := s.Store.GetThread(threadID)
	if err != nil {
		return nil, err
	}
	p, err := s.Store.GetProject(t.ProjectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(t.Cwd, source)
	}
	path, err := filepath.EvalSymlinks(source)
	if err != nil {
		return nil, err
	}
	allowed := false
	for _, root := range []string{t.Cwd, s.Layout.LibraryDir(t.ProjectID)} {
		if root == "" {
			continue
		}
		base, e := filepath.EvalSymlinks(root)
		if e != nil {
			continue
		}
		rel, e := filepath.Rel(base, path)
		if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("file is outside the thread's working directory and library")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("file must be a regular file")
	}
	a, err := s.Layout.SaveAttachment(t.ProjectID, filepath.Base(path), f, 200<<20)
	if err != nil {
		return nil, err
	}
	e := &db.ThreadEvent{ThreadID: threadID, Kind: db.EvFile, Content: caption, Payload: AttachmentsPayload([]Attachment{a})}
	if err := s.Store.AddThreadEvent(e); err != nil {
		if stored, e := s.Layout.LibraryPath(t.ProjectID, a.Path); e == nil {
			_ = os.Remove(stored)
		}
		return nil, err
	}
	s.Hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: e})
	if p.Type == db.ProjectProject && p.MainThreadID != "" && t.ID != p.MainThreadID {
		mirror := &db.ThreadEvent{ThreadID: p.MainThreadID, Kind: db.EvFile, Content: caption, Payload: e.Payload, Source: db.SourceThread, SourceRef: t.ID}
		if err := s.Store.AddThreadEvent(mirror); err == nil {
			s.Hub.Publish(Event{Type: EventThreadEvent, ProjectID: t.ProjectID, Data: mirror})
		} else {
			slog.Error("mirror published file to project conversation", "thread", t.ID, "error", err)
		}
	}
	if s.Relay != nil {
		stored, err := s.Layout.LibraryPath(t.ProjectID, a.Path)
		if err == nil {
			s.Relay.File(*t, stored, a, caption)
		}
	}
	return e, nil
}
