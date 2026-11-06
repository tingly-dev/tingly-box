package protocolserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestSpeechForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []typ.RuleScenario{typ.ScenarioTTS, typ.ScenarioOpenAI, typ.ScenarioTeam} {
		t.Run(string(scenario), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/audio/speech", r.URL.Path)
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				assert.Equal(t, "audio-test-client/1.0", r.UserAgent())
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, "upstream-tts", body["model"])
				assert.Equal(t, "hello", body["input"])
				assert.Equal(t, map[string]any{"id": "voice_custom"}, body["voice"])
				assert.Equal(t, "kept", body["vendor_option"])
				w.Header().Set("Content-Type", "audio/wav")
				w.Header().Set("X-Request-ID", "speech-123")
				_, _ = w.Write([]byte{'R', 'I', 'F', 'F', 0, 255})
			}))
			defer upstream.Close()
			router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleAnthropic, scenario, "speaker", "upstream-tts")
			for _, version := range []string{"", "/v1"} {
				req := httptest.NewRequest(http.MethodPost, "/tingly/"+string(scenario)+version+"/audio/speech", strings.NewReader(`{"model":"speaker","input":"hello","voice":{"id":"voice_custom"},"vendor_option":"kept"}`))
				req.Header.Set("User-Agent", "audio-test-client/1.0")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.Equal(t, []byte{'R', 'I', 'F', 'F', 0, 255}, w.Body.Bytes())
				assert.Equal(t, "audio/wav", w.Header().Get("Content-Type"))
				assert.Equal(t, "speech-123", w.Header().Get("X-Request-ID"))
			}
		})
	}
}

func TestSpeechValidationAndScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, setTeamIDFromHeader)
	for _, test := range []struct {
		name, scenario, body, team, want string
		status                           int
	}{
		{"invalid JSON", "tts", `{`, "", "JSON object", 400},
		{"duplicate routing model", "tts", `{"model":"allowed","model":"bypass","input":"hello","voice":"alloy"}`, "", "duplicate JSON", 400},
		{"missing input", "tts", `{"model":"m","voice":"alloy"}`, "", "input is required", 400},
		{"missing voice", "tts", `{"model":"m","input":"hello"}`, "", "voice", 400},
		{"invalid speed", "tts", `{"model":"m","input":"hello","voice":"alloy","speed":0}`, "", "speed", 400},
		{"invalid format", "tts", `{"model":"m","input":"hello","voice":"alloy","response_format":"exe"}`, "", "response_format", 400},
		{"wrong scenario", "claude_code", `{"model":"m","input":"hello","voice":"alloy"}`, "", "does not support", 400},
		{"isolated team", "team", `{"model":"m","input":"hello","voice":"alloy"}`, "team-a", "not configured", 400},
		{"body limit", "tts", strings.Repeat("x", maxSpeechRequestBytes+1), "", "maximum", 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/tingly/"+test.scenario+"/v1/audio/speech", strings.NewReader(test.body))
			r.Header.Set("X-Test-Team-ID", test.team)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			assert.Equal(t, test.status, w.Code)
			assert.Contains(t, w.Body.String(), test.want)
		})
	}
}

func TestSpeechRequiresModelAuth(t *testing.T) {
	router := gin.New()
	NewHandler(ProtocolHandlerDeps{}).RegisterRoutes(router, func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/tts/v1/audio/speech", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestSpeechUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
	}))
	defer upstream.Close()
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioTTS, "m", "upstream")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tingly/tts/audio/speech", strings.NewReader(`{"model":"m","input":"hello","voice":"alloy"}`)))
	assert.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
}

func TestSpeechStreamsBeforeUpstreamCompletes(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "last")
	}))
	defer upstream.Close()
	defer close(release)
	router := newDualProviderRouter(t, upstream.URL, protocol.APIStyleOpenAI, typ.ScenarioTTS, "m", "upstream")
	server := httptest.NewServer(router)
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(server.URL+"/tingly/tts/audio/speech", "application/json", strings.NewReader(`{"model":"m","input":"hello","voice":"alloy"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	buf := make([]byte, 5)
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	assert.Equal(t, "first", string(buf))
}

func TestAudioUsageObserver(t *testing.T) {
	o := newAudioUsageObserver("text/event-stream")
	o.Write([]byte("data: {\"type\":\"speech.audio.delta\",\"audio\":\"AAAA\"}\n\ndata: {\"usage\":{\"input_"))
	o.Write([]byte("tokens\":12,\"output_tokens\":24}}\n\n"))
	assert.Equal(t, 12, o.usage().InputTokens)
	assert.Equal(t, 24, o.usage().OutputTokens)
	// Oversized audio events never prevent parsing the later usage event.
	o.Write([]byte("data: " + strings.Repeat("a", maxAudioUsageMetadata+1) + "\n\ndata: {\"usage\":{\"input_tokens\":15}}\n\n"))
	assert.Equal(t, 15, o.usage().InputTokens)
}
