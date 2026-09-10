package imagegen

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// Handler serves /api/v1/imagegen endpoints. It carries no state: the output
// directory is derived fresh from constant.GetTinglyConfDir() on every call,
// the same way internal/protocolserver/openai_image.go's persistImages
// resolves where it writes generated images.
type Handler struct{}

// NewHandler creates a Handler.
func NewHandler() *Handler {
	return &Handler{}
}

// GetInfo handles GET /api/v1/imagegen/info: reports read-only facts about
// the imagegen scenario, currently just the directory generated/edited
// images are persisted to (~/.tingly-box/image/YYYYMMDD/*.png, see
// persistImages). Side-effect-free — this server never reaches into the
// local OS to open a file manager window; the path is handed to the
// frontend so the user can navigate there themselves, which also works
// whenever the browser isn't on the same machine as this server.
func (h *Handler) GetInfo(c *gin.Context) {
	dir := constant.GetImageDir(constant.GetTinglyConfDir())
	c.JSON(http.StatusOK, ImageGenInfoResponse{Success: true, OutputDir: dir})
}
