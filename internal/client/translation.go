package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol/translation"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TranslationHTTPError contains safe status metadata, never vendor error bodies
// which may echo credentials (LibreTranslate places its key in the JSON body).
type TranslationHTTPError struct{ Status int }

func (e *TranslationHTTPError) Error() string {
	return fmt.Sprintf("translation upstream returned HTTP %d", e.Status)
}

type TranslationClient struct {
	provider *typ.Provider
	http     *http.Client
}

func NewTranslationClient(provider *typ.Provider, sessionID typ.SessionID) *TranslationClient {
	timeout := provider.Timeout
	if timeout <= 0 {
		timeout = constant.DefaultRequestTimeout
	}
	return &TranslationClient{provider: provider, http: &http.Client{
		Transport:     providerTransportChain(createSessionBoundTransport(provider, sessionID), provider),
		Timeout:       time.Duration(timeout) * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (t *TranslationClient) Supports(source, target string) bool {
	return translation.NativeSupports(string(t.provider.APIStyle), source, target)
}

func (t *TranslationClient) Translate(ctx context.Context, in translation.TextInput) (translation.TextOutput, error) {
	out := translation.TextOutput{}
	style := string(t.provider.APIStyle)
	if !t.Supports(in.SourceLang, in.TargetLang) {
		return out, fmt.Errorf("unsupported language pair")
	}
	body, err := translation.NativeRequest(style, in)
	if err != nil {
		return out, err
	}
	path := "/translate"
	if style == "libretranslate" {
		if key := t.provider.GetAccessToken(); key != "" {
			body["api_key"] = key
		}
	} else {
		path = "/v2/translate"
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.provider.APIBase, "/")+path, bytes.NewReader(data))
	if err != nil {
		return out, fmt.Errorf("invalid translation endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	if style == "deepl" {
		req.Header.Set("Authorization", "DeepL-Auth-Key "+t.provider.GetAccessToken())
	}
	resp, err := t.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, fmt.Errorf("translation upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, &TranslationHTTPError{Status: resp.StatusCode}
	}
	const maxResponse = 4 << 20
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		return out, fmt.Errorf("invalid translation response body")
	}
	return translation.NativeResult(style, data)
}
