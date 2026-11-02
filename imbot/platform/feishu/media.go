package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Feishu's image and file APIs use different keys and message types. Other
// formats (including SVG, audio and video) are sent as ordinary files.
func feishuImage(media core.MediaAttachment) bool {
	if media.Type != "image" {
		return false
	}
	switch strings.ToLower(strings.SplitN(media.MimeType, ";", 2)[0]) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff", "image/x-icon", "image/vnd.microsoft.icon":
		return true
	default:
		return false
	}
}

func (b *Bot) sendMedia(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if b.client == nil {
		return nil, fmt.Errorf("feishu client not initialized")
	}
	var last *core.SendResult
	for _, media := range opts.Media {
		u, err := url.Parse(media.URL)
		if err != nil || u.Scheme != "file" || u.Host != "" || !filepath.IsAbs(u.Path) {
			return nil, fmt.Errorf("feishu media requires an absolute local file URL")
		}
		f, err := os.Open(u.Path)
		if err != nil {
			return nil, fmt.Errorf("open media: %w", err)
		}
		name := media.Filename
		if name == "" {
			name = filepath.Base(u.Path)
		}
		msgType, key, err := b.uploadMedia(ctx, f, name, feishuImage(media))
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close media: %w", closeErr)
		}
		payload, err := json.Marshal(map[string]string{msgType + "_key": key})
		if err != nil {
			return nil, err
		}
		req := larkim.NewCreateMessageReqBuilder().ReceiveIdType(getReceiveIdType(target)).Body(
			larkim.NewCreateMessageReqBodyBuilder().ReceiveId(target).MsgType(msgType).Content(string(payload)).Build(),
		).Build()
		resp, err := b.client.Message.Create(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("send %s: %w", msgType, err)
		}
		if !resp.Success() {
			return nil, fmt.Errorf("send %s: code=%d msg=%s", msgType, resp.Code, resp.Msg)
		}
		last = &core.SendResult{MessageID: b.extractMessageIDFromResponse(resp)}
	}
	if opts.Text != "" {
		// Feishu image/file messages do not carry captions. Send the description as
		// a separate message after the attachment, preserving the original text.
		textOpts := *opts
		textOpts.Media = nil
		if _, err := b.sendText(ctx, target, &textOpts); err != nil {
			return nil, fmt.Errorf("attachment sent but description failed: %w", err)
		}
	}
	b.UpdateLastActivity()
	return last, nil
}

func (b *Bot) uploadMedia(ctx context.Context, file *os.File, name string, image bool) (string, string, error) {
	if image {
		req := larkim.NewCreateImageReqBuilder().Body(larkim.NewCreateImageReqBodyBuilder().ImageType("message").Image(file).Build()).Build()
		resp, err := b.client.Image.Create(ctx, req)
		if err != nil {
			return "", "", fmt.Errorf("upload image: %w", err)
		}
		if !resp.Success() {
			return "", "", fmt.Errorf("upload image: code=%d msg=%s", resp.Code, resp.Msg)
		}
		if resp.Data == nil || resp.Data.ImageKey == nil || *resp.Data.ImageKey == "" {
			return "", "", fmt.Errorf("upload image: missing image key")
		}
		return "image", *resp.Data.ImageKey, nil
	}
	req := larkim.NewCreateFileReqBuilder().Body(larkim.NewCreateFileReqBodyBuilder().FileType("stream").FileName(name).File(file).Build()).Build()
	resp, err := b.client.File.Create(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("upload file: %w", err)
	}
	if !resp.Success() {
		return "", "", fmt.Errorf("upload file: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.FileKey == nil || *resp.Data.FileKey == "" {
		return "", "", fmt.Errorf("upload file: missing file key")
	}
	return "file", *resp.Data.FileKey, nil
}
