// Package translation owns the text translation contract and native MT request
// semantics. Transport, credentials and gateway routing stay outside this package.
package translation

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/openai/openai-go/v3"
	"golang.org/x/text/language"
)

const (
	HunyuanMT    = "hunyuan_mt"
	QwenMT       = "qwen_mt"
	MaxItems     = 32
	MaxTextBytes = 32 << 10
	MaxBodyBytes = 256 << 10
)

type Request struct {
	Model      string   `json:"model"`
	Text       []string `json:"text"`
	SourceLang string   `json:"source_lang"`
	TargetLang string   `json:"target_lang"`
}

type Result struct {
	Text string `json:"text"`
}

type Response struct {
	Model        string   `json:"model"`
	Translations []Result `json:"translations"`
}

// Normalize validates the small v1 contract without trimming the actual texts.
func (r *Request) Normalize() error {
	if r.Model == "" {
		r.Model = "translate"
	}
	if len(r.Text) == 0 || len(r.Text) > MaxItems {
		return fmt.Errorf("text must contain 1 to %d strings", MaxItems)
	}
	for i, s := range r.Text {
		if !utf8.ValidString(s) || strings.TrimSpace(s) == "" {
			return fmt.Errorf("text[%d] must be non-empty UTF-8 text", i)
		}
		if len(s) > MaxTextBytes {
			return fmt.Errorf("text[%d] exceeds %d UTF-8 bytes", i, MaxTextBytes)
		}
	}
	if r.SourceLang == "" {
		r.SourceLang = "auto"
	}
	var err error
	if r.SourceLang != "auto" {
		r.SourceLang, err = normalizeLanguage(r.SourceLang)
		if err != nil {
			return fmt.Errorf("source_lang: %w", err)
		}
	}
	r.TargetLang, err = normalizeLanguage(r.TargetLang)
	if err != nil {
		return fmt.Errorf("target_lang: %w", err)
	}
	return nil
}

func normalizeLanguage(s string) (string, error) {
	if s == "" || s == "auto" || len(s) > 63 || strings.Contains(s, "_") {
		return "", errors.New("expected a BCP 47 language tag")
	}
	t, err := language.Parse(s)
	if err != nil || t == language.Und {
		return "", errors.New("expected a recognized BCP 47 language tag")
	}
	// These established client aliases select a script. Other region variants
	// are retained and rejected by an adapter unless explicitly supported.
	switch t.String() {
	case "zh-CN", "zh-SG":
		return "zh-Hans", nil
	case "zh-TW":
		return "zh-Hant", nil
	}
	return t.String(), nil
}

// Names follow the shared subset of the official Hunyuan/Qwen language lists.
// New language/region support requires an explicit mapping, never base-tag fallback.
var languageNames = map[string]string{
	"zh": "Chinese", "zh-Hans": "Chinese", "zh-Hant": "Traditional Chinese",
	"en": "English", "fr": "French", "pt": "Portuguese", "es": "Spanish",
	"ja": "Japanese", "tr": "Turkish", "ru": "Russian", "ar": "Arabic",
	"ko": "Korean", "th": "Thai", "it": "Italian", "de": "German", "vi": "Vietnamese",
	"ms": "Malay", "id": "Indonesian", "fil": "Filipino", "tl": "Filipino", "hi": "Hindi",
	"pl": "Polish", "cs": "Czech", "nl": "Dutch", "km": "Khmer", "my": "Burmese",
	"fa": "Persian", "gu": "Gujarati", "ur": "Urdu", "te": "Telugu", "mr": "Marathi",
	"he": "Hebrew", "bn": "Bengali", "ta": "Tamil", "uk": "Ukrainian",
}

func Supports(backend, source, target string) bool {
	if backend != HunyuanMT && backend != QwenMT {
		return false
	}
	if languageNames[target] == "" || (source != "auto" && languageNames[source] == "") {
		return false
	}
	// The first Qwen adapter deliberately omits unverified script support.
	return backend != QwenMT || (source != "zh-Hant" && target != "zh-Hant")
}

// ChatRequest builds a fresh native request for each routed attempt. Both MT
// families use user-only messages; generic chat transforms must not inject a system role.
func ChatRequest(backend, model, text, source, target string) (*openai.ChatCompletionNewParams, error) {
	if !Supports(backend, source, target) {
		return nil, errors.New("translation backend does not support the requested language pair")
	}
	req := &openai.ChatCompletionNewParams{Model: model}
	content := text
	if backend == HunyuanMT {
		content = fmt.Sprintf("Translate the following text into %s. Note that you should only output the translated result without any additional explanation:\n%s", languageNames[target], text)
	} else {
		src := "auto"
		if source != "auto" {
			src = languageNames[source]
		}
		req.SetExtraFields(map[string]any{"translation_options": map[string]string{"source_lang": src, "target_lang": languageNames[target]}})
	}
	req.Messages = []openai.ChatCompletionMessageParamUnion{openai.UserMessage(content)}
	return req, nil
}

func CompletionResult(r *openai.ChatCompletion) (Result, error) {
	if r == nil || len(r.Choices) != 1 {
		return Result{}, errors.New("model returned an invalid translation response")
	}
	c := r.Choices[0]
	m := c.Message
	if c.FinishReason != "stop" || m.Role != "assistant" || m.Refusal != "" || len(m.ToolCalls) != 0 || m.FunctionCall.Name != "" || !utf8.ValidString(m.Content) || strings.TrimSpace(m.Content) == "" {
		return Result{}, errors.New("model did not return a complete text translation")
	}
	return Result{Text: m.Content}, nil
}
