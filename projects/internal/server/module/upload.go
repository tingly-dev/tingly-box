package module

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
)

// uploadFormOverhead is the room a request gets for the multipart framing
// around the files themselves (boundaries, part headers), not file bytes.
const uploadFormOverhead = 1 << 20

// UploadRequest is the route middleware of the multipart upload endpoints.
// maxFile is the per-file limit those endpoints enforce.
//
// It clears the connection deadlines the host's http.Server arms when the
// request starts and never extends: the read deadline (30s in Tingly Box)
// would cut an upload's body mid-transfer — a large file, or a slow link —
// and make it fail as a multipart parse error; the write deadline (10m) was
// armed even earlier than the response, which an upload can outlast. The body
// is capped at the most one message may carry (maxFile × MaxAttachments, plus
// framing) so the bytes are bounded even though the time is not: without it
// ParseMultipartForm would spool an unbounded body to disk before any
// per-file check could run.
func UploadRequest(maxFile int64) gin.HandlerFunc {
	limit := maxFile*rt.MaxAttachments + uploadFormOverhead
	return func(c *gin.Context) {
		rc := http.NewResponseController(c.Writer)
		if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logrus.WithError(err).Debug("Failed to clear connection read deadline")
		}
		if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			logrus.WithError(err).Debug("Failed to clear connection write deadline")
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// ParseUpload reads the multipart form of an upload request and returns its
// "files" field. When the form cannot be read it answers the request itself
// and reports false: 413 for a body over the request cap, 400 for a body that
// is not a multipart form, otherwise a 400 carrying the underlying reason,
// which is logged too so a failure has a cause instead of only a label.
func ParseUpload(c *gin.Context) ([]*multipart.FileHeader, bool) {
	form, err := c.MultipartForm()
	if err == nil {
		return form.File["files"], true
	}
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		apierr.Message(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("the upload is larger than %d MB", tooBig.Limit>>20))
	case errors.Is(err, http.ErrNotMultipart):
		apierr.BadRequest(c, `expected multipart form with field "files"`)
	default:
		logrus.WithError(err).WithField("path", c.Request.URL.Path).Warn("upload could not be read")
		apierr.BadRequest(c, "could not read the upload: "+err.Error())
	}
	return nil, false
}

// UploadError answers a failed upload: a file over the limit is 413 with the
// reason, anything else a 400 (the request named or carried something the
// library refused), logged with its cause.
func UploadError(c *gin.Context, err error) {
	var tooLarge *rt.TooLargeError
	if errors.As(err, &tooLarge) {
		apierr.Message(c, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	logrus.WithError(err).WithField("path", c.Request.URL.Path).Warn("upload failed")
	apierr.BadRequest(c, err.Error())
}
