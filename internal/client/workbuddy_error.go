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

// WorkBuddy errors carry code/msg/displayMsg/displayTips, not {error:{...}}.
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
		var display, tips map[string]string
		_ = json.Unmarshal(env["displayMsg"], &display)
		_ = json.Unmarshal(env["displayTips"], &tips)
		// Newer responses put the reason in displayTips and use the generic
		// "request illegal" as msg. Code 11128 alone is still ambiguous.
		details := strings.ToLower(msg + " " + display["en"])
		for _, tip := range tips {
			details += " " + strings.ToLower(tip)
		}
		channelRefused := strings.Contains(details, "unapproved channel") ||
			strings.Contains(details, "failed channel verification") ||
			strings.Contains(details, "没通过渠道校验") ||
			strings.Contains(details, "未通過通路驗證")
		kind := "workbuddy_error"
		switch {
		case resp.StatusCode == http.StatusForbidden && code == "11140":
			kind = "workbuddy_account_restricted"
			msg += ": account restricted by the upstream; submit feedback in WorkBuddy to appeal"
		case code == "11128" && channelRefused, code == "10085":
			kind = "workbuddy_channel_refused"
			msg += "; use an allowed client channel or another provider"
		}
		if display["en"] != "" {
			msg += ": " + display["en"]
		}
		if tips["en"] != "" {
			msg += ": " + tips["en"]
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
