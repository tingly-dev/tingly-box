package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// deeplClient calls the DeepL translation API.
//
// Request shape (POST {base}/v2/translate):
//
//	{"text": ["..."], "source_lang": "EN", "target_lang": "ZH"}
//
// Response shape:
//
//	{"translations": [{"detected_source_language": "EN", "text": "..."}]}
type deeplClient struct {
	provider   *typ.Provider
	model      string
	httpClient *http.Client
}

func newDeepLClient(provider *typ.Provider, model string) (*deeplClient, error) {
	return &deeplClient{
		provider:   provider,
		model:      model,
		httpClient: &http.Client{},
	}, nil
}

type deeplTranslateRequest struct {
	Text       []string `json:"text"`
	SourceLang string   `json:"source_lang,omitempty"`
	TargetLang string   `json:"target_lang"`
}

type deeplTranslateResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
}

func (c *deeplClient) Translate(ctx context.Context, req *Request) (*Response, error) {
	deeplReq := deeplTranslateRequest{
		Text:       []string{req.Input},
		TargetLang: strings.ToUpper(req.TargetLang),
	}
	if req.SourceLang != "" && req.SourceLang != "auto" {
		deeplReq.SourceLang = strings.ToUpper(req.SourceLang)
	}

	body, err := json.Marshal(deeplReq)
	if err != nil {
		return nil, fmt.Errorf("deepl translate: marshal request: %w", err)
	}

	apiBase := strings.TrimRight(c.provider.APIBase, "/")
	endpoint := fmt.Sprintf("%s/v2/translate", apiBase)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deepl translate: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := c.provider.GetAccessToken(); token != "" {
		httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepl translate: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepl translate: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepl translate: upstream %d: %s", resp.StatusCode, string(respBody))
	}

	var deeplResp deeplTranslateResponse
	if err := json.Unmarshal(respBody, &deeplResp); err != nil {
		return nil, fmt.Errorf("deepl translate: unmarshal response: %w", err)
	}
	if len(deeplResp.Translations) == 0 {
		return nil, fmt.Errorf("deepl translate: empty response")
	}

	translation := deeplResp.Translations[0].Text
	model := req.Model
	if model == "" {
		model = c.model
	}
	return &Response{
		Model:              model,
		Translation:        translation,
		DetectedSourceLang: strings.ToLower(deeplResp.Translations[0].DetectedSourceLanguage),
		Usage: Usage{
			InputCharacters:  len([]rune(req.Input)),
			OutputCharacters: len([]rune(translation)),
		},
	}, nil
}

func (c *deeplClient) Provider() *typ.Provider { return c.provider }
func (c *deeplClient) Vendor() Vendor          { return VendorDeepL }
func (c *deeplClient) Close() error            { return nil }
