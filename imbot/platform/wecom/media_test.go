package wecom

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/weixin/types"
)

type fakeOut struct {
	uploads   []*types.MediaUploadRequest
	media     []*types.OutboundMessage
	texts     []*types.OutboundMessage
	uploadErr error
}

func (f *fakeOut) Send(_ context.Context, m *types.OutboundMessage) (*types.OutboundResult, error) {
	f.texts = append(f.texts, m)
	return &types.OutboundResult{OK: true}, nil
}

func (f *fakeOut) SendMedia(_ context.Context, m *types.OutboundMessage) (*types.OutboundResult, error) {
	f.media = append(f.media, m)
	return &types.OutboundResult{OK: true}, nil
}

func (f *fakeOut) UploadMedia(_ context.Context, r *types.MediaUploadRequest) (*types.MediaUploadResult, error) {
	f.uploads = append(f.uploads, r)
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	return &types.MediaUploadResult{EncryptQuery: "media-123"}, nil
}

func newMediaBot(t *testing.T, out *fakeOut) *Bot {
	t.Helper()
	bot, err := NewBot(&core.Config{
		Platform: core.PlatformWecom,
		Auth:     core.AuthConfig{Type: "oauth", ClientID: "id", ClientSecret: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bot.out = out
	bot.MarkReady()
	return bot
}

func tempFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSendMessage_FileUploadsThenSendsWithCaption(t *testing.T) {
	out := &fakeOut{}
	bot := newMediaBot(t, out)
	p := tempFile(t, "report.pdf")

	if _, err := bot.SendMessage(context.Background(), "chat1", &core.SendMessageOptions{
		Text:     "caption",
		Metadata: map[string]interface{}{"context_token": "tok"},
		Media:    []core.MediaAttachment{{Type: "document", URL: "file://" + p, Filename: "report.pdf"}},
	}); err != nil {
		t.Fatal(err)
	}

	if len(out.uploads) != 1 || out.uploads[0].FilePath != p || out.uploads[0].MediaType != "file" {
		t.Fatalf("uploads = %+v", out.uploads)
	}
	if len(out.media) != 1 {
		t.Fatalf("media sends = %d", len(out.media))
	}
	m := out.media[0]
	if m.Metadata["wecom_media_id"] != "media-123" || m.ContentType != "file" || m.To != "chat1" || m.ContextToken != "tok" {
		t.Errorf("media msg = %+v", m)
	}
	if len(out.texts) != 1 || out.texts[0].Text != "caption" {
		t.Errorf("caption sends = %+v", out.texts)
	}
}

func TestSendMessage_ImageIsUploadedAsImage(t *testing.T) {
	out := &fakeOut{}
	bot := newMediaBot(t, out)
	png := tempFile(t, "a.PNG")
	if _, err := bot.SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "image", URL: "file://" + png, Filename: "a.PNG"}},
	}); err != nil {
		t.Fatal(err)
	}
	if out.uploads[0].MediaType != "image" || out.media[0].ContentType != "image" {
		t.Errorf("upload=%+v media=%+v", out.uploads[0], out.media[0])
	}

	// An image format WeCom does not take as image still arrives, as a file,
	// and the message type matches the uploaded type.
	out2 := &fakeOut{}
	bot2 := newMediaBot(t, out2)
	svg := tempFile(t, "a.svg")
	if _, err := bot2.SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "image", URL: "file://" + svg, Filename: "a.svg"}},
	}); err != nil {
		t.Fatal(err)
	}
	if out2.uploads[0].MediaType != "file" || out2.media[0].ContentType != "file" {
		t.Errorf("upload=%+v media=%+v", out2.uploads[0], out2.media[0])
	}
}

func TestSendMessage_PresetMediaIDSkipsUpload(t *testing.T) {
	out := &fakeOut{}
	bot := newMediaBot(t, out)
	if _, err := bot.SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Metadata: map[string]interface{}{"wecom_media_id": "pre"},
		Media:    []core.MediaAttachment{{Type: "image", Filename: "x.png"}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(out.uploads) != 0 || out.media[0].Metadata["wecom_media_id"] != "pre" {
		t.Errorf("uploads=%v media=%+v", out.uploads, out.media)
	}
}

func TestSendMessage_MediaErrorsAreSurfaced(t *testing.T) {
	p := tempFile(t, "a.txt")
	media := []core.MediaAttachment{{Type: "document", URL: "file://" + p}}

	out := &fakeOut{uploadErr: errors.New("boom")}
	if _, err := newMediaBot(t, out).SendMessage(context.Background(), "c", &core.SendMessageOptions{Media: media}); err == nil {
		t.Fatal("upload error must surface")
	}
	if len(out.media) != 0 {
		t.Error("must not send after failed upload")
	}

	if _, err := newMediaBot(t, &fakeOut{}).SendMessage(context.Background(), "c", &core.SendMessageOptions{
		Media: []core.MediaAttachment{{Type: "document", URL: "file:///no/such"}},
	}); err == nil {
		t.Fatal("missing file must surface")
	}
}
