package protocolserver

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const maxSpeechRequestBytes = 2 * 1024 * 1024

// HandleOpenAISpeech accepts the OpenAI JSON contract, including custom voice
// IDs, instructions and vendor extras. Only model is rewritten for routing.
func (ph *ProtocolHandler) HandleOpenAISpeech(c *gin.Context) {
	scenario, ok := audioScenario(c, typ.TransportTTS)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSpeechRequestBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		audioBadRequest(c, status, "could not read speech request (maximum 2 MiB)")
		return
	}
	if err := validateSpeechRequest(body); err != nil {
		audioBadRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	model := gjson.GetBytes(body, "model").String()
	provider, actualModel, ok := ph.resolveAudioRoute(c, scenario, model, true)
	if !ok {
		return
	}
	body, err = sjson.SetBytes(body, "model", actualModel)
	if err != nil {
		audioBadRequest(c, http.StatusBadRequest, err.Error())
		return
	}
	ph.forwardAudio(c, provider, actualModel, "audio/speech", "application/json", bytes.NewReader(body))
}

func validateSpeechRequest(body []byte) error {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return errors.New("request must be a JSON object")
	}
	// Duplicate JSON keys have different first/last-wins semantics across
	// parsers. Reject them so an upstream cannot see a different model than
	// the one authorized and rewritten by the routing layer.
	seen := make(map[string]bool)
	duplicate := false
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if seen[key.String()] {
			duplicate = true
			return false
		}
		seen[key.String()] = true
		return true
	})
	if duplicate {
		return errors.New("duplicate JSON fields are not allowed")
	}
	for _, field := range []string{"model", "input"} {
		value := gjson.GetBytes(body, field)
		if value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
			return errors.New(field + " is required and must be a non-empty string")
		}
	}
	voice := gjson.GetBytes(body, "voice")
	if !((voice.Type == gjson.String && strings.TrimSpace(voice.String()) != "") || (voice.IsObject() && voice.Get("id").Type == gjson.String && strings.TrimSpace(voice.Get("id").String()) != "")) {
		return errors.New("voice must be a non-empty name or an object with an id")
	}
	if speed := gjson.GetBytes(body, "speed"); speed.Exists() && (speed.Type != gjson.Number || speed.Float() < 0.25 || speed.Float() > 4) {
		return errors.New("speed must be between 0.25 and 4")
	}
	if format := gjson.GetBytes(body, "response_format"); format.Exists() {
		switch format.String() {
		case "mp3", "opus", "aac", "flac", "wav", "pcm":
		default:
			return errors.New("unsupported response_format")
		}
	}
	if format := gjson.GetBytes(body, "stream_format"); format.Exists() && format.String() != "audio" && format.String() != "sse" {
		return errors.New("stream_format must be audio or sse")
	}
	return nil
}
