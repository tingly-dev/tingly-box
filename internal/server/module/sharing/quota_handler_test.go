package sharing

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/db"
)

func TestHandlerWeeklyQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := db.NewAPITokenStore(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	h := NewHandler(s)
	r := gin.New()
	r.POST("/tokens", h.Create)
	r.GET("/tokens/:token_id", h.Get)
	r.PUT("/tokens/:token_id/quota", h.SetQuota)
	created := sharingRequest(r, http.MethodPost, "/tokens", `{"display_name":"A","weekly_token_limit":100}`)
	require.Equal(t, 201, created.Code)
	var key TokenCreateResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &key))
	record, err := s.GetToken(key.TokenID)
	require.NoError(t, err)
	require.NoError(t, s.ChargeTokens(record.ID, 50))
	path := "/tokens/" + key.TokenID
	updated := sharingRequest(r, http.MethodPut, path+"/quota", `{"weekly_token_limit":40}`)
	require.Equal(t, 200, updated.Code)
	var info APITokenInfo
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &info))
	require.Equal(t, int64(50), info.WeeklyTokensUsed)
	require.Equal(t, int64(40), *info.WeeklyTokenLimit)
	require.Nil(t, info.QuotaResetsAt) // No provider clock available in this test.
	require.Equal(t, "provider", info.QuotaReset.Mode)
	anchor := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	manual := sharingRequest(r, http.MethodPut, path+"/quota", `{"weekly_token_limit":40,"quota_reset":{"mode":"manual","anchor":"`+anchor+`"}}`)
	require.Equal(t, 200, manual.Code)
	require.NoError(t, json.Unmarshal(manual.Body.Bytes(), &info))
	require.Equal(t, int64(50), info.WeeklyTokensUsed)
	require.Equal(t, "manual", info.QuotaReset.Mode)
	require.Equal(t, anchor, info.QuotaResetsAt.UTC().Format(time.RFC3339))
	for _, policy := range []string{`{"mode":"manual"}`, `{"mode":"invalid"}`, `{"mode":"manual","anchor":"not-a-date"}`} {
		require.Equal(t, 400, sharingRequest(r, http.MethodPut, path+"/quota", `{"weekly_token_limit":40,"quota_reset":`+policy+`}`).Code)
	}
	_, err = s.ValidateToken(key.TokenID)
	var exhausted *db.TokenQuotaExceeded
	require.ErrorAs(t, err, &exhausted)
	for _, body := range []string{`{"weekly_token_limit":-1}`, `{"weekly_token_limit":1.5}`, `{"weekly_token_limit":9007199254740992}`} {
		require.Equal(t, 400, sharingRequest(r, http.MethodPut, path+"/quota", body).Code)
	}
	require.Equal(t, 200, sharingRequest(r, http.MethodPut, path+"/quota", `{"weekly_token_limit":null}`).Code)
	_, err = s.ValidateToken(key.TokenID)
	require.NoError(t, err)
	require.Equal(t, 404, sharingRequest(r, http.MethodPut, "/tokens/missing/quota", `{"weekly_token_limit":1}`).Code)
}

func TestRegenerateStartsFreshQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, err := db.NewAPITokenStore(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	limit := int64(100)
	key, err := s.CreateTokenForTeam("user", "old-key", db.DefaultTeamID, "A", "admin", nil, &limit)
	require.NoError(t, err)
	anchor := time.Now().UTC().Add(24 * time.Hour)
	require.NoError(t, s.SetWeeklyTokenLimit(key.TokenID, &limit, &db.QuotaResetPolicy{Mode: "manual", Anchor: &anchor}))
	require.NoError(t, s.ChargeTokens(key.ID, 100))
	before, err := s.GetToken(key.TokenID)
	require.NoError(t, err)
	h := NewHandler(s)
	r := gin.New()
	r.POST("/tokens/:token_id/regenerate", h.Regenerate)
	r.GET("/tokens/:token_id", h.Get)
	response := sharingRequest(r, http.MethodPost, "/tokens/old-key/regenerate", "")
	require.Equal(t, http.StatusOK, response.Code)
	var created TokenCreateResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	require.Equal(t, created.Token, created.TokenID)
	require.NotEqual(t, key.TokenID, created.TokenID)
	require.Equal(t, http.StatusNotFound, sharingRequest(r, http.MethodGet, "/tokens/old-key", "").Code)
	require.Equal(t, http.StatusOK, sharingRequest(r, http.MethodGet, "/tokens/"+created.TokenID, "").Code)
	after, err := s.ValidateToken(created.TokenID)
	require.NoError(t, err)
	require.Zero(t, after.WeeklyTokensUsed)
	require.Equal(t, before.WeeklyTokenLimit, after.WeeklyTokenLimit)
	require.Equal(t, before.QuotaResetMode, after.QuotaResetMode)
	require.Equal(t, before.QuotaResetAnchor, after.QuotaResetAnchor)
	require.Equal(t, before.QuotaResetsAt, after.QuotaResetsAt)
}
