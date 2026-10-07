package virtualserver

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"

	decisionvm "github.com/tingly-dev/tingly-box/vmodel/decision"
)

// Decisions handles POST .../v1/decisions: the model field picks a decision
// virtual model, which answers from the options in the (opaque) body. Errors
// use the OpenAI envelope so the OpenAI SDK surfaces them like any other API
// error.
func (h *Handler) Decisions(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		decisionError(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body: "+err.Error())
		return
	}
	if !gjson.ValidBytes(body) {
		decisionError(c, http.StatusBadRequest, "invalid_request_error", "Invalid request body: not valid JSON")
		return
	}
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		decisionError(c, http.StatusBadRequest, "invalid_request_error", "Model is required")
		return
	}

	vm := h.decisionReg.Get(model.String())
	if vm == nil {
		decisionError(c, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("Model not found: %s", model.String()))
		return
	}

	out, err := vm.HandleDecision(body)
	if err != nil {
		var re *decisionvm.RequestError
		if errors.As(err, &re) {
			decisionError(c, http.StatusBadRequest, "invalid_request_error", re.Message)
			return
		}
		decisionError(c, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	c.Data(http.StatusOK, "application/json", out)
}

func decisionError(c *gin.Context, status int, typ, msg string) {
	c.JSON(status, gin.H{"error": gin.H{"message": msg, "type": typ}})
}
