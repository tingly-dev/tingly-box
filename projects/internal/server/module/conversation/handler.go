// Package conversation serves standalone chats (outside any project):
// listing, editing, and the streaming chat turn.
package conversation

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/settings"
	"github.com/tingly-dev/tingly-box/swagger"
)

// ConversationListResponse lists conversations.
type ConversationListResponse struct {
	Conversations []db.Conversation `json:"conversations"`
}

// ConversationDetailResponse is a conversation with its messages.
type ConversationDetailResponse struct {
	Conversation db.Conversation `json:"conversation"`
	Messages     []db.Message    `json:"messages"`
}

// CreateConversationRequest starts a conversation.
type CreateConversationRequest struct {
	Model string `json:"model,omitempty"`
}

// UpdateConversationRequest changes the fields that are set.
type UpdateConversationRequest struct {
	Title   *string `json:"title,omitempty"`
	Starred *bool   `json:"starred,omitempty"`
	Model   *string `json:"model,omitempty"`
}

// OKResponse acknowledges a deletion.
type OKResponse struct {
	OK bool `json:"ok"`
}

// Handler is the conversation HTTP module.
type Handler struct {
	store    *db.Store
	settings *settings.Service

	busyMu sync.Mutex
	busy   map[string]bool
}

// NewHandler builds the conversation module.
func NewHandler(store *db.Store, settings *settings.Service) *Handler {
	return &Handler{store: store, settings: settings, busy: map[string]bool{}}
}

var _ module.Module = (*Handler)(nil)

// RegisterRoutes mounts conversation endpoints.
func (h *Handler) RegisterRoutes(rt *module.Routes) {
	api := rt.API
	api.GET("/conversations", h.list,
		swagger.WithTags("conversations"),
		swagger.WithDescription("List conversations, most recent first"),
		swagger.WithQuery("q", "string", "Search titles and message text"),
		swagger.WithQuery("starred", "boolean", "Only starred conversations"),
		swagger.WithQuery("limit", "integer", "Maximum number of conversations"),
		swagger.WithResponseModel(ConversationListResponse{}))
	api.POST("/conversations", h.create,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Start a standalone chat"),
		swagger.WithRequestModel(CreateConversationRequest{}),
		swagger.WithResponseModel(db.Conversation{}))
	api.GET("/conversations/:id", h.get,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Get a conversation with its messages"),
		swagger.WithResponseModel(ConversationDetailResponse{}))
	api.PATCH("/conversations/:id", h.update,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Rename, star or change the model of a chat"),
		swagger.WithRequestModel(UpdateConversationRequest{}),
		swagger.WithResponseModel(db.Conversation{}))
	api.DELETE("/conversations/:id", h.delete,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Delete a conversation"),
		swagger.WithResponseModel(OKResponse{}))
	api.POST("/conversations/:id/messages", h.send,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Send a user message and stream the reply as text/event-stream; each data line is a StreamEvent"),
		swagger.WithRequestModel(SendMessageRequest{}),
		swagger.WithResponseModel(StreamEvent{}))
	api.POST("/conversations/:id/regenerate", h.regenerate,
		swagger.WithTags("conversations"),
		swagger.WithDescription("Replace the last reply and stream a new one as text/event-stream"),
		swagger.WithRequestModel(RegenerateRequest{}),
		swagger.WithResponseModel(StreamEvent{}))
}

func (h *Handler) list(c *gin.Context) {
	limit := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	convs, err := h.store.ListConversations(db.ConversationFilter{
		Query:   c.Query("q"),
		Limit:   limit,
		Starred: c.Query("starred") == "true",
	})
	if err != nil {
		apierr.Store(c, err, "conversations")
		return
	}
	if convs == nil {
		convs = []db.Conversation{}
	}
	c.JSON(http.StatusOK, ConversationListResponse{Conversations: convs})
}

func (h *Handler) create(c *gin.Context) {
	var req CreateConversationRequest
	if !apierr.Bind(c, &req) {
		return
	}
	conv := &db.Conversation{Model: strings.TrimSpace(req.Model)}
	if err := h.store.CreateConversation(conv); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusCreated, conv)
}

func (h *Handler) get(c *gin.Context) {
	conv, err := h.store.GetConversation(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	msgs, err := h.store.ListMessages(conv.ID)
	if err != nil {
		apierr.Store(c, err, "messages")
		return
	}
	resp := ConversationDetailResponse{Conversation: *conv, Messages: msgs}
	if resp.Messages == nil {
		resp.Messages = []db.Message{}
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handler) update(c *gin.Context) {
	var req UpdateConversationRequest
	if !apierr.Bind(c, &req) {
		return
	}
	conv, err := h.store.GetConversation(c.Param("id"))
	if err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	if req.Title != nil {
		conv.Title = strings.TrimSpace(*req.Title)
	}
	if req.Starred != nil {
		conv.Starred = *req.Starred
	}
	if req.Model != nil {
		conv.Model = strings.TrimSpace(*req.Model)
	}
	// Edits are metadata: keep the conversation's place in "recent" order.
	if err := h.store.SaveConversationMeta(conv); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusOK, conv)
}

func (h *Handler) delete(c *gin.Context) {
	if err := h.store.DeleteConversation(c.Param("id")); err != nil {
		apierr.Store(c, err, "conversation")
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// acquire marks a conversation busy for one streaming turn. It reports false
// if a turn is already running.
func (h *Handler) acquire(id string) bool {
	h.busyMu.Lock()
	defer h.busyMu.Unlock()
	if h.busy[id] {
		return false
	}
	h.busy[id] = true
	return true
}

func (h *Handler) release(id string) {
	h.busyMu.Lock()
	delete(h.busy, id)
	h.busyMu.Unlock()
}
