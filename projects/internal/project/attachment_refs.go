package project

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// AttachmentRef identifies a file attached to a user message in the project conversation.
type AttachmentRef struct {
	MessageID string `json:"message_id"`
	Index     int    `json:"index"`
}

func (c *Coordinator) resolveAttachmentRefs(p *db.Project, refs []AttachmentRef) ([]Attachment, error) {
	if len(refs) > MaxAttachments {
		return nil, fmt.Errorf("at most %d attachments per message", MaxAttachments)
	}
	var atts []Attachment
	seen := make(map[string]bool)
	for _, ref := range refs {
		e, err := c.store.FindThreadEvent(p.MainThreadID, ref.MessageID)
		if err != nil || e.Kind != db.EvUser || e.Source == db.SourceThread || e.Source == db.SourceSystem {
			return nil, fmt.Errorf("attachment source %q is not a user message in this project", ref.MessageID)
		}
		files := ParseAttachments(e.Payload)
		if ref.Index < 0 || ref.Index >= len(files) {
			return nil, fmt.Errorf("attachment index %d is invalid for message %q", ref.Index, ref.MessageID)
		}
		path := files[ref.Index].Path
		if seen[path] {
			continue
		}
		resolved, err := c.layout.ResolveAttachments(p.ID, []string{path})
		if err != nil {
			return nil, err
		}
		seen[path] = true
		atts = append(atts, resolved[0])
	}
	return atts, nil
}
