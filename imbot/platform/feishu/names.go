package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Feishu events carry only ids, so a person's or a group's name has to be
// asked for. Both lookups need a scope the app owner may not have granted;
// without it a chat is simply shown by its id (design:
// .design/bot-im-alias-display.md D1). A missing scope is learned once, logged
// once, and not retried for a while — never once per message.

const (
	nameTTL         = 12 * time.Hour
	noNameTTL       = 10 * time.Minute
	failedTTL       = time.Minute
	noPermissionFor = 30 * time.Minute
	nameLookupLimit = 2 * time.Second

	scopeUserName = "contact:user.base:readonly"
	scopeChatName = "im:chat:readonly"
)

// errNoPermission marks a lookup the app is not allowed to make.
var errNoPermission = errors.New("feishu: app lacks the permission to read names")

type nameFetcher interface {
	// userName returns the person's display name; idType is "open_id" or "user_id".
	userName(ctx context.Context, idType, id string) (string, error)
	chatName(ctx context.Context, chatID string) (string, error)
}

type nameEntry struct {
	name    string
	expires time.Time
}

type nameDirectory struct {
	fetch nameFetcher
	warn  func(format string, args ...any)
	now   func() time.Time

	mu       sync.Mutex
	cache    map[string]nameEntry
	disabled map[string]time.Time // kind -> no lookups until
}

func newNameDirectory(fetch nameFetcher, warn func(string, ...any)) *nameDirectory {
	return &nameDirectory{fetch: fetch, warn: warn, now: time.Now, cache: map[string]nameEntry{}, disabled: map[string]time.Time{}}
}

// UserName is the person's name, or "" when the platform will not say.
func (d *nameDirectory) UserName(ctx context.Context, idType, id string) string {
	return d.lookup(ctx, "user", scopeUserName, idType+":"+id, func(ctx context.Context) (string, error) {
		return d.fetch.userName(ctx, idType, id)
	})
}

// ChatName is the group's title, or "".
func (d *nameDirectory) ChatName(ctx context.Context, chatID string) string {
	return d.lookup(ctx, "chat", scopeChatName, chatID, func(ctx context.Context) (string, error) {
		return d.fetch.chatName(ctx, chatID)
	})
}

func (d *nameDirectory) lookup(ctx context.Context, kind, scope, key string, call func(context.Context) (string, error)) string {
	if d == nil || key == "" || strings.HasSuffix(key, ":") {
		return ""
	}
	cacheKey := kind + "\x00" + key
	now := d.now()
	d.mu.Lock()
	if e, ok := d.cache[cacheKey]; ok && now.Before(e.expires) {
		d.mu.Unlock()
		return e.name
	}
	if until, ok := d.disabled[kind]; ok && now.Before(until) {
		d.mu.Unlock()
		return ""
	}
	d.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, nameLookupLimit)
	defer cancel()
	name, err := call(ctx)
	name = strings.TrimSpace(name)

	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case errors.Is(err, errNoPermission):
		firstTime := d.disabled[kind].IsZero()
		d.disabled[kind] = now.Add(noPermissionFor)
		if firstTime && d.warn != nil {
			d.warn("Feishu cannot read %ss' names: the app lacks scope %s. Chats are shown by their id; grant the scope in the developer console to show names.", kind, scope)
		}
		return ""
	case err != nil:
		d.cache[cacheKey] = nameEntry{expires: now.Add(failedTTL)}
		return ""
	case name == "":
		d.cache[cacheKey] = nameEntry{expires: now.Add(noNameTTL)}
	default:
		d.cache[cacheKey] = nameEntry{name: name, expires: now.Add(nameTTL)}
	}
	return name
}

// larkNameFetcher asks Feishu / Lark. Users go through a raw request so the
// bot does not link the whole contact service (see buildIMService).
type larkNameFetcher struct {
	client *larkim.V1
	config *larkcore.Config
}

// feishuPermissionCodes are the API codes meaning "this app may not read that".
var feishuPermissionCodes = map[int]bool{99991672: true, 99991679: true, 99991676: true, 41050: true, 99992402: true}

func (f larkNameFetcher) userName(ctx context.Context, idType, id string) (string, error) {
	raw, err := larkcore.Request(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   "/open-apis/contact/v3/users/:user_id",
		PathParams:                larkcore.PathParams{"user_id": id},
		QueryParams:               larkcore.QueryParams{"user_id_type": []string{idType}},
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	}, f.config)
	if err != nil {
		return "", err
	}
	if raw.StatusCode == http.StatusForbidden || raw.StatusCode == http.StatusUnauthorized {
		return "", errNoPermission
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw.RawBody, &body); err != nil {
		return "", err
	}
	if feishuPermissionCodes[body.Code] {
		return "", errNoPermission
	}
	if body.Code != 0 {
		return "", fmt.Errorf("feishu contact lookup: code %d", body.Code)
	}
	return body.Data.User.Name, nil
}

func (f larkNameFetcher) chatName(ctx context.Context, chatID string) (string, error) {
	resp, err := f.client.Chat.Get(ctx, larkim.NewGetChatReqBuilder().ChatId(chatID).Build())
	if err != nil {
		return "", err
	}
	if resp.ApiResp != nil && (resp.ApiResp.StatusCode == http.StatusForbidden || resp.ApiResp.StatusCode == http.StatusUnauthorized) {
		return "", errNoPermission
	}
	if feishuPermissionCodes[resp.Code] {
		return "", errNoPermission
	}
	if !resp.Success() {
		return "", fmt.Errorf("feishu chat lookup: code %d", resp.Code)
	}
	if resp.Data == nil || resp.Data.Name == nil {
		return "", nil
	}
	return *resp.Data.Name, nil
}
