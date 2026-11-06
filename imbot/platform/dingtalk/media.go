package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

const (
	defaultOAPIBase = "https://oapi.dingtalk.com"
	defaultAPIBase  = "https://api.dingtalk.com"

	// maxMediaBytes is the client-side cap for one upload. DingTalk's
	// media/upload documents 20MB for images and files; a larger file is
	// rejected here with a readable error instead of an opaque platform one.
	maxMediaBytes = 20 << 20
)

// conversation is what a robot needs to address a chat through the OpenAPI,
// learned from the inbound message (the stream callback is the only place
// DingTalk tells us whether a conversation is 1:1 and who the user is).
type conversation struct {
	direct  bool
	staffID string
}

// Robot message keys (https://open.dingtalk.com/document/development/robot-message-type).
const (
	msgKeyImage = "sampleImageMsg"
	msgKeyFile  = "sampleFile"
	msgKeyText  = "sampleText"
)

// dingtalkImageExts are the image formats DingTalk accepts as type=image.
// Anything else (webp, svg, ...) goes up as a plain file so it still arrives.
var dingtalkImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true}

// scrubURLError drops the request URL that net/http embeds in its errors: the
// token and upload calls carry appsecret / access_token in the query string, and
// these errors end up in logs and in the chat's error message.
func scrubURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

// sendMedia uploads each attachment to DingTalk and sends it through the robot
// OpenAPI. The stream-mode session webhook cannot carry images or files, and it
// expires, so it is not used here. Caption text follows as its own message.
func (b *Bot) sendMedia(ctx context.Context, conversationID string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if len(opts.Media) == 0 {
		return nil, core.NewBotError(core.ErrUnknown, "no media to send", false)
	}

	b.mu.RLock()
	conv, ok := b.convMap[conversationID]
	b.mu.RUnlock()
	if !ok {
		return nil, core.NewBotError(core.ErrPlatformError, "unknown DingTalk conversation; the chat must send the bot a message first", false)
	}

	var last *core.SendResult
	for _, item := range opts.Media {
		res, err := b.sendOneMedia(ctx, conversationID, conv, item)
		if err != nil {
			return nil, err
		}
		last = res
	}

	if opts.Text != "" {
		for _, chunk := range b.ChunkText(opts.Text) {
			if _, err := b.robotSend(ctx, conversationID, conv, msgKeyText, map[string]string{"content": chunk}); err != nil {
				return nil, fmt.Errorf("attachment sent but description failed: %w", err)
			}
		}
	}

	b.UpdateLastActivity()
	return last, nil
}

func (b *Bot) sendOneMedia(ctx context.Context, conversationID string, conv conversation, item core.MediaAttachment) (*core.SendResult, error) {
	path, err := core.NormalizeMediaURL(item.URL)
	if err != nil || path == "" || !filepath.IsAbs(path) {
		return nil, core.NewBotError(core.ErrMediaNotSupported, "DingTalk media requires an absolute local file path or file:// URL", false)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, core.NewBotError(core.ErrMediaNotSupported, fmt.Sprintf("open media: %v", err), false)
	}
	if info.IsDir() {
		return nil, core.NewBotError(core.ErrMediaNotSupported, "media path is a directory", false)
	}
	if info.Size() == 0 {
		return nil, core.NewBotError(core.ErrMediaNotSupported, "media file is empty", false)
	}
	if info.Size() > maxMediaBytes {
		return nil, core.NewBotError(core.ErrMediaNotSupported, fmt.Sprintf("media file is %d bytes; DingTalk allows at most %d", info.Size(), maxMediaBytes), false)
	}

	name := item.Filename
	if name == "" {
		name = filepath.Base(path)
	}
	ext := strings.ToLower(filepath.Ext(name))
	asImage := item.Type == "image" && dingtalkImageExts[ext]

	uploadType := "file"
	if asImage {
		uploadType = "image"
	}
	mediaID, err := b.uploadMedia(ctx, path, name, uploadType)
	if err != nil {
		return nil, core.WrapError(err, core.PlatformDingTalk, core.ErrMediaNotSupported)
	}

	var (
		msgKey string
		param  map[string]string
	)
	if asImage {
		msgKey, param = msgKeyImage, map[string]string{"photoURL": mediaID}
	} else {
		fileType := strings.TrimPrefix(ext, ".")
		if fileType == "" {
			fileType = "file"
		}
		msgKey, param = msgKeyFile, map[string]string{"mediaId": mediaID, "fileName": name, "fileType": fileType}
	}
	key, err := b.robotSend(ctx, conversationID, conv, msgKey, param)
	if err != nil {
		return nil, err
	}
	return &core.SendResult{MessageID: key, Timestamp: time.Now().Unix()}, nil
}

// uploadMedia posts the file to /media/upload and returns its media_id.
func (b *Bot) uploadMedia(ctx context.Context, path, name, mediaType string) (string, error) {
	token, err := b.getAccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("dingtalk upload: get token: %w", err)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("dingtalk upload: open: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// DingTalk's examples send type both in the query and as a form field.
	if err := mw.WriteField("type", mediaType); err != nil {
		return "", err
	}
	part, err := mw.CreateFormFile("media", name)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("dingtalk upload: read: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	q := url.Values{"access_token": {token}, "type": {mediaType}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.oapiBase+"/media/upload?"+q.Encode(), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("dingtalk upload: http: %w", scrubURLError(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("dingtalk upload: status %d: %s", resp.StatusCode, data)
	}

	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("dingtalk upload: decode response: %w", err)
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk upload: errcode %d: %s", out.ErrCode, out.ErrMsg)
	}
	if out.MediaID == "" {
		return "", fmt.Errorf("dingtalk upload: response has no media_id: %s", data)
	}
	return out.MediaID, nil
}

// robotSend sends one robot message to a group (groupMessages/send) or a 1:1
// chat (oToMessages/batchSend) and returns DingTalk's processQueryKey.
func (b *Bot) robotSend(ctx context.Context, conversationID string, conv conversation, msgKey string, param any) (string, error) {
	token, err := b.getAccessToken(ctx)
	if err != nil {
		return "", core.WrapError(fmt.Errorf("dingtalk send: get token: %w", err), core.PlatformDingTalk, core.ErrPlatformError)
	}
	msgParam, err := json.Marshal(param)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"robotCode": b.Config().Auth.ClientID,
		"msgKey":    msgKey,
		"msgParam":  string(msgParam),
	}
	endpoint := "/v1.0/robot/groupMessages/send"
	if conv.direct {
		if conv.staffID == "" {
			return "", core.NewBotError(core.ErrPlatformError, "unknown DingTalk user for 1:1 chat; the user must send the bot a message first", false)
		}
		endpoint = "/v1.0/robot/oToMessages/batchSend"
		payload["userIds"] = []string{conv.staffID}
	} else {
		payload["openConversationId"] = conversationID
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiBase+endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", core.WrapError(fmt.Errorf("dingtalk send: http: %w", scrubURLError(err)), core.PlatformDingTalk, core.ErrPlatformError)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", core.NewBotError(core.ErrPlatformError, fmt.Sprintf("dingtalk send %s: status %d: %s", msgKey, resp.StatusCode, data), false)
	}

	var out struct {
		ProcessQueryKey   string   `json:"processQueryKey"`
		InvalidStaffIDs   []string `json:"invalidStaffIdList"`
		FlowControlledIDs []string `json:"flowControlledStaffIdList"`
	}
	_ = json.Unmarshal(data, &out)
	if len(out.InvalidStaffIDs) > 0 || len(out.FlowControlledIDs) > 0 {
		return "", core.NewBotError(core.ErrPlatformError, fmt.Sprintf("dingtalk send %s: not delivered (invalid users %v, rate limited %v)", msgKey, out.InvalidStaffIDs, out.FlowControlledIDs), false)
	}
	if out.ProcessQueryKey == "" {
		out.ProcessQueryKey = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return out.ProcessQueryKey, nil
}

// resolveMediaURLs trades each inbound attachment's download code for a link
// the receiver can fetch (a stream callback carries only the code). A failure
// leaves the URL empty: the message still arrives, and the receiver reports
// that the file could not be downloaded.
func (b *Bot) resolveMediaURLs(ctx context.Context, robotCode string, msg *core.Message) {
	mc, ok := msg.Content.(*core.MediaContent)
	if !ok {
		return
	}
	if robotCode == "" {
		robotCode = b.Config().Auth.ClientID
	}
	for i := range mc.Media {
		code, _ := mc.Media[i].Raw[downloadCodeKey].(string)
		if code == "" {
			continue
		}
		link, err := b.downloadLink(ctx, robotCode, code)
		if err != nil {
			b.Logger().Error("Failed to get DingTalk download link: %v", err)
			continue
		}
		mc.Media[i].URL = link
	}
}

// downloadLink calls robot/messageFiles/download for a download code.
func (b *Bot) downloadLink(ctx context.Context, robotCode, downloadCode string) (string, error) {
	token, err := b.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(map[string]string{"downloadCode": downloadCode, "robotCode": robotCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiBase+"/v1.0/robot/messageFiles/download", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", scrubURLError(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, data)
	}
	var out struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.DownloadURL == "" {
		return "", fmt.Errorf("response has no downloadUrl: %s", data)
	}
	return out.DownloadURL, nil
}
