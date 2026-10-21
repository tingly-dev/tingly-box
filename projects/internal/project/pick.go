package project

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Connecting an IM chat from the chat itself (.design/projects-im.md, "M5 的落地"):
// /connect offers the projects and folders to talk to, and a pick binds the
// chat, as binding it on the web does.

// channelTargetsMax caps the projects and folders offered to a chat.
const channelTargetsMax = 10

// ChannelTarget is a project or folder a chat may be connected to.
type ChannelTarget struct {
	ProjectID string
	Name      string
	// Folder is set for a folder; Path is its folder.
	Folder bool
	Path   string
}

// ChannelTargets lists what a chat may be connected to: the projects and
// folders that are not archived, starred and recent first. A folder is
// offered once it has a thread, since a chat binds to a thread.
func (s *Service) ChannelTargets() ([]ChannelTarget, error) {
	ps, err := s.Store.ListProjects("", false)
	if err != nil {
		return nil, err
	}
	out := []ChannelTarget{}
	for _, p := range ps {
		if len(out) == channelTargetsMax {
			break
		}
		if p.Type == db.ProjectFolder {
			if threads, err := s.Store.ListThreads(p.ID); err != nil || len(threads) == 0 {
				continue
			}
		}
		out = append(out, ChannelTarget{ProjectID: p.ID, Name: p.Name, Folder: p.Type == db.ProjectFolder, Path: p.Path})
	}
	return out, nil
}

// ConnectChannel binds a chat to the project or folder it picked. A project
// binds it to the project conversation. A folder binds it to the folder's
// latest thread with a new session on its next message (as after /new), so
// the chat starts fresh rather than walking into someone's session. reply
// is what to tell the chat.
func (s *Service) ConnectChannel(platform, chatID, projectID string) (reply string, err error) {
	p, err := s.Store.GetProject(projectID)
	if err != nil {
		return "", err
	}
	if p.Type != db.ProjectFolder {
		if _, err := s.BindChannel(platform, chatID, "", "", p.ID); err != nil {
			return "", err
		}
		return fmt.Sprintf("This chat now talks to the project conversation of %s. /help lists what it can do.", p.Name), nil
	}
	threads, err := s.Store.ListThreads(p.ID)
	if err != nil {
		return "", err
	}
	if len(threads) == 0 {
		return "", fmt.Errorf("%s has no session yet; start one in Projects on the web", p.Path)
	}
	b, err := s.BindChannel(platform, chatID, "", threads[0].ID, "")
	if err != nil {
		return "", err
	}
	if err := s.Store.SetChannelBindingNewThread(b.ID, true); err != nil {
		return "", err
	}
	return fmt.Sprintf("This chat now works in %s; your next message starts a new session there. /help lists what it can do.", p.Path), nil
}
