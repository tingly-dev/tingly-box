package remoteagent_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
)

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

// A file keeps the name its sender gave it all the way to Projects, and an
// image that has no name gets a readable one, never the platform's media id.
func Test_BoundChatFilesKeepTheirNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cd":
			w.Header().Set("Content-Disposition", `attachment; filename="invoice 2026.pdf"`)
			_, _ = w.Write([]byte("%PDF-1.4"))
		default:
			_, _ = w.Write(pngBytes)
		}
	}))
	defer srv.Close()
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	names := func() []string {
		var out []string
		for _, in := range router.inbound() {
			for _, f := range in.Files {
				out = append(out, f.Name)
			}
		}
		return out
	}
	send := func(m imbot.MediaAttachment, want int) []string {
		chat.SendMedia([]imbot.MediaAttachment{m})
		require.Eventually(t, func() bool { return len(router.inbound()) == want }, 3*time.Second, 10*time.Millisecond)
		return names()
	}

	got := send(imbot.MediaAttachment{Type: "image", URL: srv.URL + "/img/AgACAgIAAxkBAAIBZ2lkxyz", MimeType: "image/png"}, 1)
	assert.Regexp(t, regexp.MustCompile(`^image-\d{8}-\d{6}\.png$`), got[0], "an unnamed image is named by when it came, not by its media id")

	got = send(imbot.MediaAttachment{Type: "document", URL: srv.URL + "/cd", MimeType: "application/pdf"}, 2)
	assert.Equal(t, "invoice 2026.pdf", got[1], "the download's Content-Disposition names a file whose message had no name")

	got = send(imbot.MediaAttachment{Type: "document", URL: srv.URL + "/x", MimeType: "image/png", Filename: "季度报告 final.png"}, 3)
	assert.Equal(t, "季度报告 final.png", got[2])

	got = send(imbot.MediaAttachment{Type: "document", URL: srv.URL + "/x", MimeType: "image/png", Filename: `..\..\etc/pass wd.png`}, 4)
	assert.Equal(t, "pass wd.png", got[3], "directories are dropped from a sender's name")

	got = send(imbot.MediaAttachment{Type: "document", URL: srv.URL + "/x", MimeType: "image/png", Filename: "scan"}, 5)
	assert.Equal(t, "scan.png", got[4], "a name with no extension gets the one the bytes show")
}
