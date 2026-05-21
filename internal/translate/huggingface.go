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

// huggingfaceClient calls the HuggingFace Inference API for dedicated
// translation models (Helsinki-NLP/opus-mt-*, NLLB-200, M2M-100, etc.).
//
// Request shape (POST {base}/models/{model}):
//
//	{"inputs": "...", "parameters": {"src_lang": "en", "tgt_lang": "zh"}}
//
// Response shape:
//
//	[{"translation_text": "..."}]
type huggingfaceClient struct {
	provider   *typ.Provider
	model      string
	httpClient *http.Client
}

func newHuggingFaceClient(provider *typ.Provider, model string) (*huggingfaceClient, error) {
	return &huggingfaceClient{
		provider:   provider,
		model:      model,
		httpClient: &http.Client{},
	}, nil
}

type hfTranslateRequest struct {
	Inputs     string        `json:"inputs"`
	Parameters hfParameters  `json:"parameters,omitempty"`
}

type hfParameters struct {
	SrcLang string `json:"src_lang,omitempty"`
	TgtLang string `json:"tgt_lang,omitempty"`
}

type hfTranslateResponse []struct {
	TranslationText string `json:"translation_text"`
}

func (c *huggingfaceClient) Translate(ctx context.Context, req *Request) (*Response, error) {
	hfReq := hfTranslateRequest{
		Inputs: req.Input,
	}
	src := req.SourceLang
	if src == "auto" {
		src = ""
	}
	if src != "" || req.TargetLang != "" {
		hfReq.Parameters = hfParameters{
			SrcLang: src,
			TgtLang: req.TargetLang,
		}
	}

	body, err := json.Marshal(hfReq)
	if err != nil {
		return nil, fmt.Errorf("huggingface translate: marshal request: %w", err)
	}

	model := req.Model
	if model == "" {
		model = c.model
	}
	apiBase := strings.TrimRight(c.provider.APIBase, "/")
	endpoint := fmt.Sprintf("%s/models/%s", apiBase, model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("huggingface translate: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token := c.provider.GetAccessToken(); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("huggingface translate: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("huggingface translate: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("huggingface translate: upstream %d: %s", resp.StatusCode, string(respBody))
	}

	var hfResp hfTranslateResponse
	if err := json.Unmarshal(respBody, &hfResp); err != nil {
		return nil, fmt.Errorf("huggingface translate: unmarshal response: %w", err)
	}
	if len(hfResp) == 0 {
		return nil, fmt.Errorf("huggingface translate: empty response")
	}

	translation := hfResp[0].TranslationText
	return &Response{
		Model:       model,
		Translation: translation,
		Usage: Usage{
			InputCharacters:  len([]rune(req.Input)),
			OutputCharacters: len([]rune(translation)),
		},
	}, nil
}

func (c *huggingfaceClient) Provider() *typ.Provider { return c.provider }
func (c *huggingfaceClient) Vendor() Vendor          { return VendorHuggingFace }
func (c *huggingfaceClient) Close() error            { return nil }
