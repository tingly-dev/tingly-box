package protocolserver

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func audioUpload(t *testing.T, fields map[string][]string, audio []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	for key, values := range fields {
		for _, value := range values {
			require.NoError(t, writer.WriteField(key, value))
		}
	}
	if audio != nil {
		part, err := writer.CreateFormFile("file", "speech.webm")
		require.NoError(t, err)
		_, err = part.Write(audio)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body, writer.FormDataContentType()
}

func TestRecognitionForwardingAndTempCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	// This exceeds the in-memory multipart threshold, proving the request
	// cleans up disk-spooled input after forwarding.
	audio := bytes.Repeat([]byte{0, 127, 255}, audioFormMemory/3+10)
	for _, scenario := range []typ.RuleScenario{typ.ScenarioASR, typ.ScenarioOpenAI, typ.ScenarioTeam} {
		for _, endpoint := range []string{"transcriptions", "translations"} {
			t.Run(string(scenario)+"/"+endpoint, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v1/audio/"+endpoint, r.URL.Path)
					assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
					require.NoError(t, r.ParseMultipartForm(audioFormMemory))
					defer r.MultipartForm.RemoveAll()
					assert.Equal(t, []string{"upstream-asr"}, r.MultipartForm.Value["model"])
					assert.Equal(t, []string{"word", "segment"}, r.MultipartForm.Value["timestamp_granularities[]"])
					assert.Equal(t, []string{"keep-me"}, r.MultipartForm.Value["vendor_option"])
					file, header, err := r.FormFile("file")
					require.NoError(t, err)
					defer file.Close()
					assert.Equal(t, "speech.webm", header.Filename)
					got, err := io.ReadAll(file)
					require.NoError(t, err)
					assert.Equal(t, audio, got)
					w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
					_, _ = io.WriteString(w, "WEBVTT\n\n00:00.000 --> 00:01.000\nHello\n")
				}))
				defer upstream.Close()
				router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic, scenario, "listener", "upstream-asr")
				body, contentType := audioUpload(t, map[string][]string{"model": {"listener"}, "timestamp_granularities[]": {"word", "segment"}, "vendor_option": {"keep-me"}}, audio)
				r := httptest.NewRequest(http.MethodPost, "/tingly/"+string(scenario)+"/v1/audio/"+endpoint, body)
				r.Header.Set("Content-Type", contentType)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.Equal(t, "WEBVTT\n\n00:00.000 --> 00:01.000\nHello\n", w.Body.String())
				assert.Equal(t, "text/vtt; charset=utf-8", w.Header().Get("Content-Type"))
				entries, err := os.ReadDir(tmp)
				require.NoError(t, err)
				for _, entry := range entries {
					assert.False(t, strings.HasPrefix(entry.Name(), "multipart-"), entry.Name())
				}
			})
		}
	}
}

func TestRecognitionValidationAndScope(t *testing.T) {
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)
	for _, test := range []struct {
		name, scenario, team, want string
		fields                     map[string][]string
		audio                      []byte
	}{
		{"missing file", "asr", "", "audio file", map[string][]string{"model": {"m"}}, nil},
		{"empty file", "asr", "", "must not be empty", map[string][]string{"model": {"m"}}, []byte{}},
		{"missing model", "asr", "", "model is required", nil, []byte("a")},
		{"duplicate model", "asr", "", "model is required", map[string][]string{"model": {"m", "x"}}, []byte("a")},
		{"invalid stream", "asr", "", "stream", map[string][]string{"model": {"m"}, "stream": {"maybe"}}, []byte("a")},
		{"wrong transport", "tts", "", "does not support", map[string][]string{"model": {"m"}}, []byte("a")},
		{"team isolation", "team", "another-team", "not configured", map[string][]string{"model": {"m"}}, []byte("a")},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, contentType := audioUpload(t, test.fields, test.audio)
			r := httptest.NewRequest(http.MethodPost, "/tingly/"+test.scenario+"/audio/transcriptions", body)
			r.Header.Set("Content-Type", contentType)
			r.Header.Set("X-Test-Team-ID", test.team)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Contains(t, w.Body.String(), test.want)
		})
	}
}

func TestRecognitionUploadLimitsAndCleanup(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, func(c *gin.Context) { c.Next() })
	for _, size := range []int{maxAudioUploadBytes + 1, maxAudioFormBytes + 1} {
		body, contentType := audioUpload(t, map[string][]string{"model": {"m"}}, bytes.Repeat([]byte("a"), size))
		r := httptest.NewRequest(http.MethodPost, "/tingly/asr/audio/transcriptions", body)
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
		entries, err := os.ReadDir(tmp)
		require.NoError(t, err)
		assert.Empty(t, entries)
	}
}

func TestRecognitionStreamingAndCancellation(t *testing.T) {
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"transcript.text.delta\",\"delta\":\"hello\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioASR, "m", "upstream")
	server := httptest.NewServer(router)
	defer server.Close()
	body, contentType := audioUpload(t, map[string][]string{"model": {"m"}, "stream": {"true"}}, []byte("audio"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/tingly/asr/audio/transcriptions", body)
	require.NoError(t, err)
	r.Header.Set("Content-Type", contentType)
	resp, err := server.Client().Do(r)
	require.NoError(t, err)
	defer resp.Body.Close()
	buf := make([]byte, 6)
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	assert.Equal(t, "data: ", string(buf))
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request was not canceled")
	}
}

func TestRecognitionRequiresModelAuthAndMultipart(t *testing.T) {
	for _, endpoint := range []string{"transcriptions", "translations"} {
		router := gin.New()
		NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, func(c *gin.Context) { c.AbortWithStatus(401) })
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/asr/audio/"+endpoint, strings.NewReader(`{}`)))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	}
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, func(c *gin.Context) { c.Next() })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/asr/audio/transcriptions", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRecognitionEarlyUpstreamFailureCleansUpload(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject before reading the upload: the multipart writer must still
		// unblock and release its file, rather than leaking a goroutine.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid key","type":"authentication_error"}}`)
	}))
	defer upstream.Close()
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioASR, "m", "upstream")
	body, contentType := audioUpload(t, map[string][]string{"model": {"m"}}, bytes.Repeat([]byte("a"), audioFormMemory+1))
	r := httptest.NewRequest(http.MethodPost, "/tingly/asr/audio/transcriptions", body)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), "multipart-"), entry.Name())
	}
}
