package smart_mem

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const descriptionMaxRunes = 120

// Handler exposes smart_mem's persist/retrieve endpoints.
type Handler struct {
	store  *FileStore
	router Router
}

// NewHandler constructs a Handler. The router defaults to a UUIDRouter
// over the given store when nil.
func NewHandler(store *FileStore, router Router) *Handler {
	if router == nil {
		router = NewUUIDRouter(store)
	}
	return &Handler{store: store, router: router}
}

// Persist handles POST /api/v1/smart_mem.
func (h *Handler) Persist(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "smart_mem store not available"})
		return
	}

	var req PersistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	raw, err := json.Marshal(req.Payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := uuid.NewString()
	if err := h.store.Put(id, raw); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, PersistResponse{
		UUID:        id,
		Description: deriveDescription(raw),
		SizeBytes:   len(raw),
	})
}

// Retrieve handles GET /api/v1/smart_mem/:uuid.
// The persisted document is returned verbatim as application/json so
// the caller gets the same shape they sent in.
func (h *Handler) Retrieve(c *gin.Context) {
	if h.router == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "smart_mem router not available"})
		return
	}
	key := c.Param("uuid")
	raw, err := h.router.Resolve(key)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

// deriveDescription produces a short preview from the raw JSON bytes:
// collapse whitespace and truncate to descriptionMaxRunes runes.
func deriveDescription(raw []byte) string {
	var b strings.Builder
	b.Grow(len(raw))
	prevSpace := false
	for _, r := range string(raw) {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	s := strings.TrimSpace(b.String())
	runes := []rune(s)
	if len(runes) > descriptionMaxRunes {
		return string(runes[:descriptionMaxRunes]) + "..."
	}
	return s
}
