package protocolserver

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	maxAudioUploadBytes = 25 * 1024 * 1024
	maxAudioFormBytes   = 32 * 1024 * 1024
	audioFormMemory     = 4 * 1024 * 1024
)

func (ph *ProtocolHandler) HandleOpenAITranscription(c *gin.Context) {
	ph.handleAudioRecognition(c, "audio/transcriptions")
}

func (ph *ProtocolHandler) HandleOpenAITranslation(c *gin.Context) {
	ph.handleAudioRecognition(c, "audio/translations")
}

func (ph *ProtocolHandler) handleAudioRecognition(c *gin.Context, path string) {
	scenario, ok := audioScenario(c, typ.TransportASR)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAudioFormBytes)
	err := c.Request.ParseMultipartForm(audioFormMemory)
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, multipart.ErrMessageTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		audioBadRequest(c, status, "expected multipart/form-data containing an audio file (maximum request size 32 MiB)")
		return
	}
	form := c.Request.MultipartForm
	if err := validateAudioForm(form); err != nil {
		status := http.StatusBadRequest
		if form != nil && len(form.File["file"]) == 1 && form.File["file"][0].Size > maxAudioUploadBytes {
			status = http.StatusRequestEntityTooLarge
		}
		audioBadRequest(c, status, err.Error())
		return
	}
	model := form.Value["model"][0]
	streamed := len(form.Value["stream"]) == 1 && form.Value["stream"][0] == "true"
	provider, actualModel, ok := ph.resolveAudioRoute(c, scenario, model, streamed)
	if !ok {
		return
	}
	body, contentType, cleanup, err := audioMultipartBody(form, actualModel)
	if err != nil {
		audioBadRequest(c, http.StatusBadRequest, "could not read uploaded audio file")
		return
	}
	defer cleanup()
	ph.forwardAudio(c, provider, actualModel, path, contentType, body)
}

func validateAudioForm(form *multipart.Form) error {
	if form == nil {
		return errors.New("multipart form is required")
	}
	models := form.Value["model"]
	if len(models) != 1 || strings.TrimSpace(models[0]) == "" {
		return errors.New("exactly one non-empty model is required")
	}
	files := form.File["file"]
	if len(files) != 1 || len(form.File) != 1 || len(form.Value["file"]) != 0 {
		return errors.New("exactly one audio file named file is required")
	}
	if files[0].Size == 0 {
		return errors.New("audio file must not be empty")
	}
	if files[0].Size > maxAudioUploadBytes {
		return errors.New("audio file exceeds the 25 MiB limit")
	}
	if values := form.Value["stream"]; len(values) > 1 || (len(values) == 1 && values[0] != "true" && values[0] != "false") {
		return errors.New("stream must be true or false")
	}
	if values := form.Value["response_format"]; len(values) > 0 {
		if len(values) != 1 {
			return errors.New("exactly one response_format is allowed")
		}
		switch values[0] {
		case "json", "verbose_json", "text", "srt", "vtt", "diarized_json":
		default:
			return errors.New("unsupported response_format")
		}
	}
	return nil
}

// audioMultipartBody streams the spooled upload into a new multipart body,
// replacing only model. Repeated fields (timestamps/include/vendor options)
// and the file's MIME type survive. cleanup joins the writer before the
// handler removes its temporary files, including on early upstream failure.
func audioMultipartBody(form *multipart.Form, model string) (io.Reader, string, func(), error) {
	header := form.File["file"][0]
	file, err := header.Open()
	if err != nil {
		return nil, "", nil, err
	}
	reader, writer := io.Pipe()
	multi := multipart.NewWriter(writer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer file.Close()
		err := func() error {
			if err := multi.WriteField("model", model); err != nil {
				return err
			}
			for name, values := range form.Value {
				if name == "model" {
					continue
				}
				for _, value := range values {
					if err := multi.WriteField(name, value); err != nil {
						return err
					}
				}
			}
			partHeader := make(textproto.MIMEHeader)
			partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": header.Filename}))
			contentType := header.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			partHeader.Set("Content-Type", contentType)
			part, err := multi.CreatePart(partHeader)
			if err != nil {
				return err
			}
			if _, err := io.Copy(part, file); err != nil {
				return fmt.Errorf("copy audio: %w", err)
			}
			return multi.Close()
		}()
		_ = writer.CloseWithError(err)
	}()
	return reader, multi.FormDataContentType(), func() { _ = reader.Close(); <-done }, nil
}
