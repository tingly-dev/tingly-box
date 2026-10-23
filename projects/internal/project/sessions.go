package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/engine"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Folder sessions: Claude Code working in a plain folder, with no project,
// coordinator, worktree or merge. A
// session is a thread with an empty ProjectID whose Worktree is the folder,
// so it shares the thread runtime and its page. See
// .design/projects-desk-absorption.md.

// PermissionModes lists the Claude Code permission modes a session can run in.
var PermissionModes = []string{"default", "acceptEdits", "auto", "plan", "dontAsk", "bypassPermissions"}

// ValidPermissionMode reports whether m is empty (inherit) or a known mode.
func ValidPermissionMode(m string) bool {
	if m == "" {
		return true
	}
	for _, v := range PermissionModes {
		if v == m {
			return true
		}
	}
	return false
}

// Errors for folder sessions.
var (
	ErrNotFolder        = errors.New("the path is not a folder")
	ErrTaskNotRunning   = errors.New("the task is not running")
	ErrBadPermission    = errors.New("unknown permission mode")
	ErrBadAutoAnswer    = errors.New("unknown auto-answer setting")
	errFolderRequired   = errors.New("folder is required")
	ErrNotFolderProject = errors.New("the project is not a folder")
	errBadTaskID        = errors.New("bad task id")
	ErrTaskOutputUnset  = errors.New("no output recorded for that task")
)

// FolderThreadSpec describes a new thread in a folder project.
type FolderThreadSpec struct {
	ProjectID string
	Task      string
	// Attachments are uploads of the session pool sent with the first message.
	Attachments    []Attachment
	Title          string
	Model          string
	PermissionMode string
	// Profile is the host's launch profile (see Launcher); empty for its default.
	Profile string
	// Source is where the first message came from; zero for the web.
	Source Source
}

// CleanFolder resolves a folder path the user gave (~ expanded, absolute,
// cleaned) and checks it is an existing directory.
func CleanFolder(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", errFolderRequired
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: %q is not an absolute path", ErrNotFolder, raw)
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %q", ErrNotFolder, raw)
	}
	return path, nil
}

// StartFolderThread creates a thread in a folder project and runs its first
// turn.
func (m *Threads) StartFolderThread(spec FolderThreadSpec) (*db.Thread, error) {
	p, err := m.store.GetProject(spec.ProjectID)
	if err != nil {
		return nil, err
	}
	if p.Type != db.ProjectFolder {
		return nil, ErrNotFolderProject
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if p.Paused {
		return nil, ErrPaused
	}
	folder, err := CleanFolder(p.Path)
	if err != nil {
		return nil, err
	}
	task := strings.TrimSpace(spec.Task)
	if task == "" && len(spec.Attachments) == 0 {
		return nil, errors.New("the first message is empty")
	}
	if !ValidPermissionMode(spec.PermissionMode) {
		return nil, ErrBadPermission
	}
	profile, model := strings.TrimSpace(spec.Profile), strings.TrimSpace(spec.Model)
	if l, ok := m.env.(Launcher); ok {
		if err := l.CheckLaunch(context.Background(), profile, model); err != nil {
			return nil, err
		}
	} else if profile != "" {
		return nil, ErrNoProfiles
	}
	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = firstLine(task, 60)
	}
	if title == "" {
		title = spec.Attachments[0].Name
	}
	t := &db.Thread{
		ProjectID: p.ID, Agent: db.AgentClaudeCode,
		Title: title, Task: task, Status: db.ThreadWorking, Origin: "user",
		Cwd: folder, SessionID: uuid.NewString(),
		Model: model, PermissionMode: spec.PermissionMode, Profile: profile,
		HeadBranch: HeadBranch(context.Background(), folder),
	}
	if err := m.store.CreateThread(t); err != nil {
		return nil, err
	}
	if err := m.layout.AdoptUploads(DraftOwner, SessionOwner(t.ID), spec.Attachments); err != nil {
		logrus.WithError(err).Warn("move folder thread uploads")
	}
	_ = m.store.TouchProject(p.ID)
	m.publish(t)
	m.enqueue(t.ID, queued{text: task, src: spec.Source, atts: spec.Attachments})
	return t, nil
}

// SetPermissionMode sets the mode the session's next turn runs in. A
// resident process keeps the mode it was launched with, and the engine
// reopens it with --resume when the mode differs.
func (m *Threads) SetPermissionMode(id, mode string) (*db.Thread, error) {
	if !ValidPermissionMode(mode) {
		return nil, ErrBadPermission
	}
	if _, err := m.store.GetThread(id); err != nil {
		return nil, err
	}
	out, err := m.update(id, func(t *db.Thread) { t.PermissionMode = mode })
	if err == nil {
		m.addEvent(out, &db.ThreadEvent{Kind: db.EvSystem, Content: "permission mode: " + orDefault(mode)})
	}
	return out, err
}

// SetAutoAnswer sets when the thread's questions with a suggested answer
// are answered without the user. A question already waiting is timed
// again under the new setting.
func (m *Threads) SetAutoAnswer(id, mode string) (*db.Thread, error) {
	if !ValidAutoAnswer(mode) {
		return nil, ErrBadAutoAnswer
	}
	if _, err := m.store.GetThread(id); err != nil {
		return nil, err
	}
	out, err := m.update(id, func(t *db.Thread) { t.AutoAnswer = mode })
	if err == nil {
		m.prompter(id).retime()
	}
	return out, err
}

// BackgroundTask is a task a session's Claude Code process reports as running.
type BackgroundTask struct {
	TaskID      string `json:"task_id"`
	TaskType    string `json:"task_type"`
	Description string `json:"description"`
}

// BackgroundTasks lists what the thread's process runs in the background.
// The set dies with the process.
func (m *Threads) BackgroundTasks(threadID string) []BackgroundTask {
	live := m.engine.BackgroundTasks(m.sessionOf(threadID))
	out := make([]BackgroundTask, 0, len(live))
	for _, t := range live {
		out = append(out, BackgroundTask{TaskID: t.TaskID, TaskType: t.TaskType, Description: t.Description})
	}
	return out
}

// StopTask stops one background task of a session.
func (m *Threads) StopTask(ctx context.Context, threadID, taskID string) error {
	if err := m.engine.StopTask(ctx, m.sessionOf(threadID), taskID); errors.Is(err, engine.ErrTaskNotRunning) {
		return ErrTaskNotRunning
	} else {
		return err
	}
}

// TaskOutput reads the end of a background task's output file. Only the file
// Claude Code reported for that task in this session is read.
func (m *Threads) TaskOutput(threadID, taskID string, tail int) (engine.TaskOutput, error) {
	if !engine.ValidTaskID(taskID) {
		return engine.TaskOutput{}, errBadTaskID
	}
	events, err := m.store.ListThreadEvents(threadID)
	if err != nil {
		return engine.TaskOutput{}, err
	}
	path := ""
	for i := len(events) - 1; i >= 0 && path == ""; i-- {
		if events[i].Kind == db.EvTask {
			path, _ = engine.TaskOutputFile(engine.KindTask, []byte(events[i].Payload), taskID)
		}
	}
	if path == "" {
		return engine.TaskOutput{}, ErrTaskOutputUnset
	}
	return engine.ReadTaskOutput(path, taskID, tail)
}
