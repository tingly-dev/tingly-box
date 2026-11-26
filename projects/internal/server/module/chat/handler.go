// Package chat serves the read-only chats: listing, creating, attachments,
// export and deletion. The conversation itself (events, send, stop, retry) is
// served by the thread module by thread id, exactly like any session.
package chat

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/swagger"
)

// Handler serves the chats.
type Handler struct{ svc *rt.Service }

// NewHandler builds the module.
func NewHandler(svc *rt.Service) *Handler { return &Handler{svc: svc} }

var _ module.Module = (*Handler)(nil)

// ChatListResponse is the chat list and whether chats are sandboxed here.
type ChatListResponse struct {
	Chats     []db.Thread `json:"chats"`
	Sandboxed bool        `json:"sandboxed"`
}

// NewChatRequest creates a chat and runs its first message (JSON form).
type NewChatRequest struct {
	Content string `json:"content"`
	Title   string `json:"title,omitempty"`
	Model   string `json:"model,omitempty"`
}

// ChatAttachmentsResponse lists what an upload stored.
type ChatAttachmentsResponse struct {
	Attachments []rt.Attachment `json:"attachments"`
}

// ChatResponse is one chat and whether it runs sandboxed here.
type ChatResponse struct {
	Thread    db.Thread `json:"thread"`
	Sandboxed bool      `json:"sandboxed"`
}

// ChatErrorResponse is the error shape (module/apierr).
type ChatErrorResponse struct {
	Error string `json:"error"`
}

// RegisterRoutes mounts the chat endpoints.
func (h *Handler) RegisterRoutes(r *module.Routes) {
	api := r.API
	tag := swagger.WithTags("chats")
	api.GET("/chats", h.list, tag,
		swagger.WithDescription("The read-only chats, most recently updated first, and whether they run sandboxed here"),
		swagger.WithResponseModel(ChatListResponse{}))
	api.POST("/chats", h.create, tag,
		swagger.WithDescription(`Start a chat with its first message: JSON {content,title,model} or multipart (content,title,model, files "files")`),
		swagger.WithRequestModel(NewChatRequest{}),
		swagger.WithResponseModel(db.Thread{}))
	api.GET("/chats/:id", h.get, tag,
		swagger.WithDescription("One chat and whether chats run sandboxed here"),
		swagger.WithResponseModel(ChatResponse{}))
	api.POST("/chats/:id/attachments", h.uploadAttachments, tag,
		swagger.WithDescription(`Upload text or image files for a chat message (multipart field "files"); other types are refused`),
		swagger.WithResponseModel(ChatAttachmentsResponse{}))
	api.GET("/chats/:id/attachments", h.downloadAttachment, tag,
		swagger.WithDescription("Download a chat attachment by its path"),
		swagger.WithQuery("path", "string", "The attachment path returned by the upload"))
	api.GET("/chats/:id/export", h.export, tag,
		swagger.WithDescription("Export a chat as a zip: chat.json (thread and events) and every attachment, not the transcript"))
	api.DELETE("/chats/:id", h.deleteChat, tag,
		swagger.WithDescription("Delete a chat for good: stops it, removes its rows and its folder (transcripts, attachments, exports)"))
}

func (h *Handler) list(c *gin.Context) {
	chats, err := h.svc.Threads.ListChats()
	if err != nil {
		apierr.Store(c, err, "chats")
		return
	}
	c.JSON(http.StatusOK, ChatListResponse{Chats: chats, Sandboxed: h.svc.Threads.ChatSandboxed(c.Request.Context())})
}

func (h *Handler) get(c *gin.Context) {
	t, ok := h.chat(c)
	if !ok {
		return
	}
	h.svc.Threads.AnnotateOne(t)
	c.JSON(http.StatusOK, ChatResponse{Thread: *t, Sandboxed: h.svc.Threads.ChatSandboxed(c.Request.Context())})
}

func (h *Handler) create(c *gin.Context) {
	var spec rt.ChatThreadSpec
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/") {
		form, err := c.MultipartForm()
		if err != nil {
			apierr.BadRequest(c, `expected multipart form with fields content, title, model and "files"`)
			return
		}
		spec.Text = first(form.Value["content"])
		spec.Title = first(form.Value["title"])
		spec.Model = first(form.Value["model"])
		spec.Attachments, err = readFiles(form.File["files"])
		if err != nil {
			apierr.BadRequest(c, err.Error())
			return
		}
	} else {
		var req NewChatRequest
		if !apierr.Bind(c, &req) {
			return
		}
		spec.Text, spec.Title, spec.Model = req.Content, req.Title, req.Model
	}
	t, err := h.svc.Threads.StartChat(c.Request.Context(), spec)
	if err != nil {
		chatError(c, err)
		return
	}
	h.svc.Threads.AnnotateOne(t)
	c.JSON(http.StatusCreated, t)
}

func (h *Handler) uploadAttachments(c *gin.Context) {
	t, ok := h.chat(c)
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		apierr.BadRequest(c, `expected multipart form with field "files"`)
		return
	}
	uploads, err := readFiles(form.File["files"])
	if err != nil {
		apierr.BadRequest(c, err.Error())
		return
	}
	atts, err := h.svc.Threads.SaveChatAttachments(t.ID, uploads)
	if err != nil {
		chatError(c, err)
		return
	}
	c.JSON(http.StatusCreated, ChatAttachmentsResponse{Attachments: atts})
}

func (h *Handler) downloadAttachment(c *gin.Context) {
	t, ok := h.chat(c)
	if !ok {
		return
	}
	rel := c.Query("path")
	f, info, err := h.svc.Layout.OpenChatAttachment(t.ID, rel)
	if err != nil {
		apierr.Message(c, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	module.ServeDownload(c, f, info, filepath.Base(rel))
}

func (h *Handler) export(c *gin.Context) {
	t, ok := h.chat(c)
	if !ok {
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="`+t.ID+`.zip"`)
	if err := h.svc.Threads.ExportChat(t.ID, c.Writer); err != nil {
		// Headers may already be written; the client sees a truncated zip.
		return
	}
}

func (h *Handler) deleteChat(c *gin.Context) {
	t, ok := h.chat(c)
	if !ok {
		return
	}
	if err := h.svc.Threads.DeleteChat(t.ID); err != nil {
		chatError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// chat loads the thread named by :id and checks it is a chat.
func (h *Handler) chat(c *gin.Context) (*db.Thread, bool) {
	t, err := h.svc.Store.GetThread(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "chat")
		return nil, false
	}
	if !h.svc.Threads.IsChat(t.ProjectID) {
		apierr.Message(c, http.StatusNotFound, "chat not found")
		return nil, false
	}
	return t, true
}

func chatError(c *gin.Context, err error) {
	var limit *rt.ChatLimitError
	switch {
	case errors.Is(err, rt.ErrChatUnsupportedType),
		errors.Is(err, rt.ErrChatTooMany),
		errors.Is(err, rt.ErrChatQuota),
		errors.Is(err, rt.ErrChatCount),
		errors.As(err, &limit):
		apierr.BadRequest(c, err.Error())
	default:
		apierr.Store(c, err, "chat")
	}
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// readFiles reads the multipart files into memory, capped so a huge upload is
// not buffered. Per-kind and total limits are checked when they are stored.
func readFiles(headers []*multipart.FileHeader) ([]rt.ChatUpload, error) {
	out := make([]rt.ChatUpload, 0, len(headers))
	var total int64
	for _, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, rt.ChatMaxTextBytes+1))
		f.Close()
		if err != nil {
			return nil, err
		}
		total += int64(len(data))
		if total > rt.ChatMaxMessageAttachmentBytes+1 {
			return nil, errors.New("attachments are too large for one message")
		}
		out = append(out, rt.ChatUpload{Name: fh.Filename, Data: data})
	}
	return out, nil
}
