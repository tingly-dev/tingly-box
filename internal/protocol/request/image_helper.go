package request

import "strings"

// ParseImageURLToAnthropicSource splits a string from OpenAI's image_url.url
// (used by both Chat Completions and the Responses API) into the bits an
// Anthropic image source needs.
//
// Returns mediaType and data when the URL is a "data:<mime>;base64,<payload>"
// data URL. Returns the same URL string back as remoteURL when it's a regular
// http(s) URL. Exactly one of (mediaType+data) or remoteURL is non-empty for
// well-formed input.
func ParseImageURLToAnthropicSource(url string) (mediaType, data, remoteURL string) {
	if url == "" {
		return "", "", ""
	}
	if !strings.HasPrefix(url, "data:") {
		return "", "", url
	}
	rest := strings.TrimPrefix(url, "data:")
	semi := strings.IndexByte(rest, ';')
	comma := strings.IndexByte(rest, ',')
	if semi < 0 || comma < 0 || semi > comma {
		// Malformed data URL — give the caller back the raw string so it can
		// at least be passed through as a reference rather than dropped.
		return "", "", url
	}
	mediaType = rest[:semi]
	// Encoding token between ';' and ',' — we only support base64 data URLs.
	encoding := rest[semi+1 : comma]
	if encoding != "base64" {
		return "", "", url
	}
	data = rest[comma+1:]
	return mediaType, data, ""
}
