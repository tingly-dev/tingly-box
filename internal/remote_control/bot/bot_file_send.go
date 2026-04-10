package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	imbot "github.com/tingly-dev/tingly-box/imbot/core"
)

// imageExtensionsForSend is the set of extensions sent as "image" type.
var imageExtensionsForSend = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
	".webp": {},
}

// detectSendMediaType returns "image" for image extensions, "document" otherwise.
func detectSendMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := imageExtensionsForSend[ext]; ok {
		return "image"
	}
	return "document"
}

// SendFile sends a local file to the user via the IM bot.
// The file is read from disk and sent as a MediaAttachment.
// caption may be empty.
func (h *BotHandler) SendFile(ctx context.Context, hCtx HandlerContext, filePath, caption string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot access file '%s': %w", filePath, err)
	}

	mediaType := detectSendMediaType(filePath)
	filename := filepath.Base(filePath)

	attachment := imbot.MediaAttachment{
		Type:     mediaType,
		URL:      "file://" + filePath,
		Filename: filename,
		Size:     info.Size(),
	}

	opts := &imbot.SendMessageOptions{
		Text:  caption,
		Media: []imbot.MediaAttachment{attachment},
	}

	// Forward context_token from incoming message metadata (required by Weixin)
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = ct
		}
	}

	_, err = hCtx.Bot.SendMessage(ctx, hCtx.ChatID, opts)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"chatID":   hCtx.ChatID,
			"filePath": filePath,
		}).Warn("Failed to send file")
		return fmt.Errorf("failed to send file: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"chatID":    hCtx.ChatID,
		"filePath":  filePath,
		"mediaType": mediaType,
		"size":      info.Size(),
	}).Info("File sent successfully")

	return nil
}
