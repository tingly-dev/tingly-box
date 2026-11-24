package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/translation"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestTranslationNativeErrorsAndMissingUsage(t *testing.T) {
	for _, tc := range []struct {
		style, body string
		status      int
		wantErr     bool
	}{
		{"libretranslate", `{"translatedText":"你好"}`, 200, false},
		{"libretranslate", `{"error":"provider-key"}`, 200, true},
		{"libretranslate", `{"translatedText":["wrong shape"]}`, 200, true},
		{"libretranslate", `{"translatedText":" "}`, 200, true},
		{"deepl", `{"translations":[]}`, 200, true},
		{"deepl", `{"translations":[{"text":"one"},{"text":"two"}]}`, 200, true},
		{"deepl", `{"translations":[{"text":"one","billed_characters":-1}]}`, 200, true},
		{"deepl", `{"translations":[{"text":"one"}]}`, 200, false},
		{"libretranslate", `{"error":"provider-key"}`, 401, true},
		{"deepl", `{"error":"provider-key"}`, 429, true},
	} {
		t.Run(tc.style+tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			c := NewTranslationClient(&typ.Provider{UUID: t.Name(), APIStyle: protocol.APIStyle(tc.style), APIBase: server.URL, Token: "provider-key"}, typ.SessionID{})
			out, err := c.Translate(context.Background(), translation.TextInput{Text: "hello", SourceLang: "auto", TargetLang: "zh"})
			if tc.wantErr {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "provider-key")
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, out.Tokens)
			require.Nil(t, out.Result.BilledCharacters)
			if tc.status != 200 {
				var status *TranslationHTTPError
				require.True(t, errors.As(err, &status))
				require.Equal(t, tc.status, status.Status)
			}
		})
	}
}

func TestTranslationNativeCancellationAndRedirect(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
		defer server.Close()
		c := NewTranslationClient(&typ.Provider{UUID: t.Name(), APIStyle: protocol.APIStyleLibreTranslate, APIBase: server.URL}, typ.SessionID{})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := c.Translate(ctx, translation.TextInput{Text: "hello", SourceLang: "auto", TargetLang: "en"})
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
	t.Run("redirect does not forward credentials", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("must not follow redirect") }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
		defer server.Close()
		c := NewTranslationClient(&typ.Provider{UUID: t.Name(), APIStyle: protocol.APIStyleLibreTranslate, APIBase: server.URL, Token: "secret"}, typ.SessionID{})
		_, err := c.Translate(context.Background(), translation.TextInput{Text: "hello", SourceLang: "en", TargetLang: "fr"})
		require.Error(t, err)
		require.False(t, strings.Contains(err.Error(), "secret"))
	})
}
