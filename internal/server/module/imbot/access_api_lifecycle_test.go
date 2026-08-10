package imbot

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/access"
)

func putCapabilityForTest(t *testing.T, router http.Handler, botUUID string, capability access.CapabilityName, enabled bool) *httptest.ResponseRecorder {
	t.Helper()
	body := []byte(`{"enabled":false}`)
	if enabled {
		body = []byte(`{"enabled":true}`)
	}
	req := httptest.NewRequest(http.MethodPut, "/bots/"+botUUID+"/capabilities/"+string(capability), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestPutCapabilityReconcilesBotEnabledState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stores, err := db.NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stores.Close() })

	botSettings, err := stores.ImBotSettings().CreateSettings(db.Settings{
		Name: "lifecycle", Platform: "telegram", AuthType: "token",
		Auth: map[string]string{"token": "test"}, Enabled: true,
	})
	require.NoError(t, err)
	require.NoError(t, stores.BotAccess().PutCapability(context.Background(), access.BotCapability{
		BotUUID: botSettings.UUID, Name: access.CapabilityRemoteControl, Enabled: true,
	}))

	handler := &Handler{store: stores.ImBotSettings(), accessStore: stores.BotAccess()}
	router := gin.New()
	router.PUT("/bots/:bot/capabilities/:capability", handler.PutCapability)

	// The last consumer going away turns the otherwise-unused Bot off.
	response := putCapabilityForTest(t, router, botSettings.UUID, access.CapabilityRemoteControl, false)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	stored, err := stores.ImBotSettings().GetSettingsByUUID(botSettings.UUID)
	require.NoError(t, err)
	require.False(t, stored.Enabled)

	// The first consumer coming back starts the Bot again.
	response = putCapabilityForTest(t, router, botSettings.UUID, access.CapabilityNotify, true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	stored, err = stores.ImBotSettings().GetSettingsByUUID(botSettings.UUID)
	require.NoError(t, err)
	require.True(t, stored.Enabled)

	// Disabling one consumer must preserve an explicit Bot-off gate while a
	// different consumer remains configured.
	require.NoError(t, stores.BotAccess().PutCapability(context.Background(), access.BotCapability{
		BotUUID: botSettings.UUID, Name: access.CapabilityRemoteControl, Enabled: true,
	}))
	require.NoError(t, stores.ImBotSettings().SetEnabled(botSettings.UUID, false))
	response = putCapabilityForTest(t, router, botSettings.UUID, access.CapabilityRemoteControl, false)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	stored, err = stores.ImBotSettings().GetSettingsByUUID(botSettings.UUID)
	require.NoError(t, err)
	require.False(t, stored.Enabled)
}
