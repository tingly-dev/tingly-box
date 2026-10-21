package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
)

// WorkBuddy's errors use {code,msg,displayMsg}, not OpenAI's {error:{...}}.
// Normalize at this provider's transport boundary, preserving the envelope for
// diagnostics and the HTTP status for callers. The SDK can then retain both the
// vendor code and message through streaming and complete-call paths.
func workbuddyNormalizeError(resp *http.Response) error {
	if resp == nil || resp.Body == nil || resp.StatusCode < 400 {
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	body := raw
	var env map[string]json.RawMessage
	if json.Unmarshal(raw, &env) == nil && env["error"] == nil && env["code"] != nil {
		code := strings.Trim(string(env["code"]), `"`)
		var msg, message string
		_ = json.Unmarshal(env["msg"], &msg)
		_ = json.Unmarshal(env["message"], &message)
		if msg == "" {
			msg = message
		}
		var display map[string]string
		_ = json.Unmarshal(env["displayMsg"], &display)
		kind := "workbuddy_error"
		switch {
		case resp.StatusCode == http.StatusForbidden && code == "11140":
			kind = "workbuddy_account_restricted"
			msg += ": account restricted by the upstream; submit feedback in WorkBuddy to appeal"
		case code == "11128" && strings.Contains(strings.ToLower(msg), "unapproved channel"), code == "10085":
			kind = "workbuddy_channel_refused"
			msg += "; use an allowed client channel or another provider"
		}
		if display["en"] != "" {
			msg += ": " + display["en"]
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		env["error"], _ = json.Marshal(map[string]any{"code": code, "type": kind, "message": msg, "vendor_error": json.RawMessage(raw)})
		if normalized, marshalErr := json.Marshal(env); marshalErr == nil {
			body = normalized
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

// IsWorkBuddyChannelRefusal identifies a non-retryable HTTP status that can use
// the existing provider fallback. Numeric codes alone are not enough.
func IsWorkBuddyChannelRefusal(err error) bool {
	var sdkErr *openai.Error
	return errors.As(err, &sdkErr) && sdkErr.StatusCode == http.StatusBadRequest && sdkErr.Type == "workbuddy_channel_refused"
}
