package project

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// UploadsDir is where everything a person attaches lives, inside the
// project library: one folder per upload, so a file can be listed, opened
// and cleaned up on its own, and the Library tab shows it.
const UploadsDir = "uploads"

// SessionOwner is where a folder thread's uploads lived before they were
// shared per folder. Nothing is written there any more; files already in it
// stay readable (see AttachmentPath).
func SessionOwner(sessionID string) string { return "_sessions/" + sessionID }

// UploadOwner is whose uploads a thread reads and writes: its project's. A
// folder project keeps its library under the data root, outside the folder
// it works in (nothing lands in the person's repository), and all of the
// folder's threads share it.
func UploadOwner(_ *db.Project, t *db.Thread) string { return t.ProjectID }

// AttachmentPath resolves an attachment of thread t: in the project's
// library, or, for a folder thread from before uploads were shared, in the
// thread's own legacy area.
func (l Layout) AttachmentPath(p *db.Project, t *db.Thread, rel string) (string, error) {
	path, err := l.LibraryPath(UploadOwner(p, t), rel)
	if err != nil {
		return "", err
	}
	if p.Type != db.ProjectFolder {
		return path, nil
	}
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	legacy, err := l.LibraryPath(SessionOwner(t.ID), rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(legacy); err == nil {
		return legacy, nil
	}
	return path, nil
}

// AttachmentsResponse lists the files stored for a message.
type AttachmentsResponse struct {
	Attachments []Attachment `json:"attachments"`
}

// An upload is one file, `uploads/<id>-<name>`: the id keeps names unique, the
// name stays readable for the model, and removing the file removes the upload.
const idLen = 12

var uploadNameRe = regexp.MustCompile(`^[0-9a-f]{12}-.+$`)

// maxNameBytes keeps id and name within a file name limit.
const maxNameBytes = 200

// MaxAttachments caps the files on one message.
const MaxAttachments = 20

// Attachment is a file a person attached to a message.
type Attachment struct {
	Name string `json:"name"`
	// Path is relative to the library root, forward slashes, always under uploads/.
	Path string `json:"path"`
	// Type is the MIME type.
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// SaveAttachment stores src as a new upload of the project. limit caps its
// size (0 for none).
func (l Layout) SaveAttachment(projectID, name string, src io.Reader, limit int64) (Attachment, error) {
	name = cleanUploadName(name)
	var id [idLen / 2]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Attachment{}, err
	}
	rel := UploadsDir + "/" + hex.EncodeToString(id[:]) + "-" + name
	dst, err := l.LibraryPath(projectID, rel)
	if err != nil {
		return Attachment{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Attachment{}, err
	}
	out, err := os.Create(dst)
	if err != nil {
		return Attachment{}, err
	}
	if limit > 0 {
		src = io.LimitReader(src, limit+1)
	}
	n, err := io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && limit > 0 && n > limit {
		err = fmt.Errorf("%s is larger than %d MB", name, limit>>20)
	}
	if err != nil {
		_ = os.Remove(dst)
		return Attachment{}, err
	}
	return Attachment{Name: name, Path: rel, Type: sniffType(dst, name), Size: n}, nil
}

// ResolveAttachments checks that each path is an existing upload of the
// project and returns what is on disk, so a request cannot name a file
// outside uploads/ or lie about its size.
func (l Layout) ResolveAttachments(projectID string, paths []string) ([]Attachment, error) {
	if len(paths) > MaxAttachments {
		return nil, fmt.Errorf("at most %d attachments per message", MaxAttachments)
	}
	out := make([]Attachment, 0, len(paths))
	for _, rel := range paths {
		rel = filepath.ToSlash(filepath.Clean(rel))
		if filepath.ToSlash(filepath.Dir(rel)) != UploadsDir || !uploadNameRe.MatchString(filepath.Base(rel)) {
			return nil, fmt.Errorf("invalid attachment %q", rel)
		}
		abs, err := l.LibraryPath(projectID, rel)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(abs)
		if err != nil || st.IsDir() {
			return nil, fmt.Errorf("attachment %q not found", rel)
		}
		name := filepath.Base(rel)[idLen+1:]
		out = append(out, Attachment{Name: name, Path: rel, Type: sniffType(abs, name), Size: st.Size()})
	}
	return out, nil
}

// AttachmentsPayload is the message payload that records attachments.
func AttachmentsPayload(atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}
	return jsonString(map[string]any{"attachments": atts})
}

// ParseAttachments reads attachments back from a message payload.
func ParseAttachments(payload string) []Attachment {
	if payload == "" {
		return nil
	}
	var pl struct {
		Attachments []Attachment `json:"attachments"`
	}
	_ = json.Unmarshal([]byte(payload), &pl)
	return pl.Attachments
}

// AttachmentNote tells a model where the attached files are on disk. A file
// that was removed since is still listed, as removed.
func AttachmentNote(libraryDir string, atts []Attachment) string {
	if len(atts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[The user attached these files; read them from disk]\n")
	for _, a := range atts {
		abs := filepath.Join(libraryDir, filepath.FromSlash(a.Path))
		if _, err := os.Stat(abs); err != nil {
			fmt.Fprintf(&b, "- %s (removed)\n", a.Name)
			continue
		}
		fmt.Fprintf(&b, "- %s (%s, %s)\n", abs, a.Type, humanSize(a.Size))
	}
	return strings.TrimRight(b.String(), "\n")
}

// withAttachments appends the attachment note to a message's text.
func withAttachments(text, libraryDir string, atts []Attachment) string {
	note := AttachmentNote(libraryDir, atts)
	if note == "" {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return note
	}
	return text + "\n\n" + note
}

func cleanUploadName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '/' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "file"
	}
	if len(name) > maxNameBytes {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		stem := strings.ToValidUTF8(name[:maxNameBytes-len(ext)], "")
		name = stem + ext
	}
	return name
}

func sniffType(abs, name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return strings.SplitN(t, ";", 2)[0]
	}
	f, err := os.Open(abs)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return strings.SplitN(http.DetectContentType(buf[:n]), ";", 2)[0]
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// SaveUploads stores the files of a multipart form field as uploads of owner.
func (l Layout) SaveUploads(owner string, files []*multipart.FileHeader, limit int64) ([]Attachment, error) {
	if len(files) == 0 || len(files) > MaxAttachments {
		return nil, fmt.Errorf("send 1 to %d files", MaxAttachments)
	}
	out := make([]Attachment, 0, len(files))
	for _, fh := range files {
		if fh.Size > limit {
			return nil, fmt.Errorf("%s is larger than %d MB", fh.Filename, limit>>20)
		}
		src, err := fh.Open()
		if err != nil {
			return nil, err
		}
		a, err := l.SaveAttachment(owner, fh.Filename, src, limit)
		src.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
