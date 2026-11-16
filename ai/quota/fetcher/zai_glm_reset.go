package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/quota"
)

// GLM Coding Plan reset cards. The endpoints are the BigModel console's own
// (not documented, not in the public API reference); the contract below comes
// from community clients that use them (see .design/provider-rewards.md).
const (
	glmRoot          = "https://open.bigmodel.cn"
	glmResetListPath = "/api/biz/customer-package-reset/list?targetType=PERSONAL"
	glmResetUsePath  = "/api/biz/customer-package-reset/use"
)

// glmResetCard is one reset card. Field names are not documented, so the
// reader below accepts the spellings seen in the wild and keeps what it does
// not recognise out of the decision.
type glmResetCard struct {
	ID        string // record id, as sent back to the use endpoint
	idNumeric bool   // the upstream id is a number and is sent as one
	ResetType string // upstream's own value, sent back unchanged
	Scope     string // "weekly" or "5h", for display and confirmation
	ExpiresAt time.Time
	Available bool
}

// glmResetEnvelope is the common {code, msg, success, data} wrapper.
type glmResetEnvelope struct {
	Code    int             `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Success *bool           `json:"success"`
	Data    json.RawMessage `json:"data"`
}

func (e glmResetEnvelope) ok() bool {
	if e.Success != nil {
		return *e.Success
	}
	return e.Code == 0 || e.Code == 200
}

func (e glmResetEnvelope) text() string {
	if e.Msg != "" {
		return e.Msg
	}
	return e.Message
}

// parseGLMResetCards reads the list response. An unrecognised shape is an
// error, not "no cards": showing nothing for a card the user holds is the
// exact failure this reader exists to prevent.
func parseGLMResetCards(body []byte, now time.Time) ([]glmResetCard, error) {
	var env glmResetEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode reset cards: %w", err)
	}
	if !env.ok() {
		return nil, fmt.Errorf("reset cards: %s", env.text())
	}
	rows, err := glmResetRows(env.Data)
	if err != nil {
		return nil, err
	}
	cards := make([]glmResetCard, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, readGLMResetCard(row, now))
	}
	return cards, nil
}

// glmResetRows finds the array of cards: data itself, or the first array
// under a list-like key of data.
func glmResetRows(data json.RawMessage) ([]map[string]any, error) {
	if len(bytes.TrimSpace(data)) == 0 || string(bytes.TrimSpace(data)) == "null" {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err == nil {
		return rows, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("reset cards: unexpected data shape")
	}
	for _, key := range []string{"list", "records", "rows", "items", "resets", "data"} {
		if raw, ok := obj[key]; ok {
			if err := json.Unmarshal(raw, &rows); err == nil {
				return rows, nil
			}
		}
	}
	return nil, fmt.Errorf("reset cards: unexpected data shape")
}

func readGLMResetCard(row map[string]any, now time.Time) glmResetCard {
	card := glmResetCard{}
	for _, key := range []string{"recordId", "id", "record_id"} {
		if v, ok := row[key]; ok && v != nil {
			switch id := v.(type) {
			case float64:
				card.ID = strconv.FormatInt(int64(id), 10)
				card.idNumeric = true
			case string:
				card.ID = id
				_, err := strconv.ParseInt(id, 10, 64)
				card.idNumeric = err == nil
			}
			break
		}
	}
	card.ResetType = glmString(row, "resetType", "type", "reset_type")
	upper := strings.ToUpper(card.ResetType)
	switch {
	case strings.Contains(upper, "WEEK"):
		card.Scope = "weekly"
	case strings.Contains(upper, "FIVE") || strings.Contains(upper, "5"):
		card.Scope = "5h"
	default:
		card.Scope = strings.ToLower(card.ResetType)
	}
	card.ExpiresAt = glmTime(row, "expireTime", "expiredTime", "expiresAt", "expireAt", "endTime", "validEndTime", "expire_time")

	status := strings.ToUpper(glmString(row, "status", "state", "useStatus"))
	spent := false
	switch status { // whole words: "UNUSED" is not "USED"
	case "USED", "EXPIRED", "INVALID", "CONSUMED", "REVOKED":
		spent = true
	}
	card.Available = card.ID != "" && !spent && (card.ExpiresAt.IsZero() || card.ExpiresAt.After(now))
	return card
}

func glmString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := row[key].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return strconv.FormatInt(int64(v), 10)
		}
	}
	return ""
}

// glmTime reads an instant given as unix seconds, unix milliseconds, RFC 3339
// or the console's "2006-01-02 15:04:05" (UTC+8, as the console shows it).
func glmTime(row map[string]any, keys ...string) time.Time {
	cst := time.FixedZone("CST", 8*3600)
	for _, key := range keys {
		switch v := row[key].(type) {
		case float64:
			if v > 1e11 {
				return time.UnixMilli(int64(v))
			}
			if v > 0 {
				return time.Unix(int64(v), 0)
			}
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
			if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, cst); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

// glmResetRequest sends one reset-card request. The key goes in the
// authorization header bare, without "Bearer": that is how the console's own
// requests carry it, unlike the monitor endpoint the quota fetch uses.
func (f *GLMFetcher) glmResetRequest(ctx context.Context, provider *ai.Provider, method, path string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint(f.baseURL, glmRoot, path), body)
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", provider.GetAccessToken())
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := quota.NewHTTPClient(provider.ProxyURL, 30*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, data, nil
}

func (f *GLMFetcher) listResetCards(ctx context.Context, provider *ai.Provider) ([]glmResetCard, error) {
	status, body, err := f.glmResetRequest(ctx, provider, http.MethodGet, glmResetListPath, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("reset cards: status %d", status)
	}
	return parseGLMResetCards(body, time.Now())
}

// addResetCards puts each available card beside the other one-shot rewards
// (Codex's reset credits do the same): a card is spent, not refilled, so it
// carries no reset time and its expiry is when it is lost.
func addGLMResetCards(usage *quota.ProviderUsage, cards []glmResetCard) {
	for _, c := range cards {
		if !c.Available {
			continue
		}
		label := "Weekly reset card"
		if c.Scope == "5h" {
			label = "5-hour reset card"
		} else if c.Scope != "weekly" {
			label = "Reset card"
		}
		// Only available cards are listed, so "available" is not repeated here.
		desc := ""
		if !c.ExpiresAt.IsZero() {
			desc = fmt.Sprintf("Expire %s", c.ExpiresAt.Format("2006-01-02"))
		}
		bd := usage.AddBreakdown("reset-card-"+c.ID, label, "resource", &quota.UsageWindow{
			Type:        quota.WindowTypeBalance,
			Used:        0,
			Limit:       1,
			Unit:        quota.UsageUnitCredits,
			Label:       "available",
			Description: desc,
		})
		bd.Redeem = &quota.UsageRedeem{ID: c.ID, Scope: c.Scope}
	}
}

// Redeem spends one reset card. It is irreversible, so: the id must be an
// available card in a fresh list; the request is sent once and never retried;
// and an answer that is not a clear yes or no is "unconfirmed", never a guess.
func (f *GLMFetcher) Redeem(ctx context.Context, provider *ai.Provider, rewardID string) (string, error) {
	if err := f.Validate(provider); err != nil {
		return "", err
	}
	cards, err := f.listResetCards(ctx, provider)
	if err != nil {
		return "", fmt.Errorf("read reset cards before spending: %w", err)
	}
	var card *glmResetCard
	for i := range cards {
		if cards[i].ID == rewardID && cards[i].Available {
			card = &cards[i]
			break
		}
	}
	if card == nil {
		return "", quota.ErrRewardNotFound
	}

	var recordID any = card.ID
	if card.idNumeric {
		n, _ := strconv.ParseInt(card.ID, 10, 64)
		recordID = n
	}
	status, body, err := f.glmResetRequest(ctx, provider, http.MethodPost, glmResetUsePath, map[string]any{
		"targetType": "PERSONAL",
		"resetType":  card.ResetType,
		"recordId":   recordID,
		"grantType":  "DIRECT",
		"requestId":  uuid.NewString(),
	})
	if err != nil {
		// Sent, outcome unknown.
		return "", fmt.Errorf("%w: %v", quota.ErrRedeemUnconfirmed, err)
	}
	switch {
	case status >= 500:
		return "", fmt.Errorf("%w: status %d", quota.ErrRedeemUnconfirmed, status)
	case status != http.StatusOK:
		return "", fmt.Errorf("%w: status %d", quota.ErrRedeemRejected, status)
	}
	var env glmResetEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("%w: unreadable answer", quota.ErrRedeemUnconfirmed)
	}
	if !env.ok() {
		// HTTP 200 with success:false is a business refusal (a spent card).
		return "", fmt.Errorf("%w: %s", quota.ErrRedeemRejected, env.text())
	}
	logrus.WithField("provider_uuid", provider.UUID).Info("glm reset card spent")
	return env.text(), nil
}

var _ quota.Redeemer = (*GLMFetcher)(nil)
