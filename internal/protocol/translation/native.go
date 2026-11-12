package translation

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// Native codecs own semantic conversion only. URL paths, credentials,
// connection pooling, proxies and deadlines remain in internal/client.
func NativeSupports(style, source, target string) bool {
	if target == "auto" || target == "" {
		return false
	}
	_, a := NativeLanguage(style, source)
	_, b := NativeLanguage(style, target)
	if style == "deepl" && source != "auto" && strings.Contains(source, "-") {
		return false
	}
	return a == nil && b == nil
}

func NativeRequest(style string, in TextInput) (map[string]any, error) {
	if !NativeSupports(style, in.SourceLang, in.TargetLang) {
		return nil, errors.New("unsupported language pair")
	}
	source, _ := NativeLanguage(style, in.SourceLang)
	target, _ := NativeLanguage(style, in.TargetLang)
	if style == "libretranslate" {
		return map[string]any{"q": in.Text, "source": source, "target": target, "format": "text"}, nil
	}
	body := map[string]any{"text": []string{in.Text}, "target_lang": target}
	if source != "" {
		body["source_lang"] = source
	}
	return body, nil
}

func NativeResult(style string, data []byte) (TextOutput, error) {
	out := TextOutput{}
	if !utf8.Valid(data) {
		return out, errors.New("invalid translation response encoding")
	}
	switch style {
	case "libretranslate":
		var result struct {
			Text     string `json:"translatedText"`
			Error    string `json:"error"`
			Detected *struct {
				Language string `json:"language"`
			} `json:"detectedLanguage"`
		}
		if err := json.Unmarshal(data, &result); err != nil || result.Error != "" {
			return out, errors.New("invalid LibreTranslate response")
		}
		out.Result.Text = result.Text
		if result.Detected != nil {
			out.Result.DetectedSourceLanguage = result.Detected.Language
		}
	case "deepl":
		var result struct {
			Translations []struct {
				Text   string `json:"text"`
				Source string `json:"detected_source_language"`
				Billed *int64 `json:"billed_characters"`
			} `json:"translations"`
		}
		if err := json.Unmarshal(data, &result); err != nil || len(result.Translations) != 1 {
			return out, errors.New("invalid DeepL response")
		}
		item := result.Translations[0]
		if item.Billed != nil && *item.Billed < 0 {
			return out, errors.New("invalid billed character count")
		}
		out.Result = Result{Text: item.Text, DetectedSourceLanguage: item.Source, BilledCharacters: item.Billed}
	default:
		return out, errors.New("unsupported native translation protocol")
	}
	if strings.TrimSpace(out.Result.Text) == "" {
		return out, errors.New("translation upstream returned empty text")
	}
	if out.Result.DetectedSourceLanguage != "" {
		tag, err := NormalizeLanguage(out.Result.DetectedSourceLanguage)
		if err != nil {
			return out, errors.New("invalid detected source language")
		}
		out.Result.DetectedSourceLanguage = tag
	}
	return out, nil
}
