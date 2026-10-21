package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// An install chatting through Remote gets Projects turned on once; a switch
// someone set, either way, is left alone.
func TestProjectsIsTurnedOnForBotsInUse(t *testing.T) {
	newServer := func(t *testing.T, bots bool) *Server {
		cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
		require.NoError(t, err)
		if bots {
			_, err := cfg.StoreManager().ImBotSettings().CreateSettings(db.Settings{Name: "tg", Platform: "telegram", AuthType: "token", Auth: map[string]string{"token": "x"}, Enabled: true})
			require.NoError(t, err)
		}
		return &Server{config: cfg}
	}
	on := func(s *Server) bool { return s.config.GetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionProjects) }

	s := newServer(t, false)
	s.adoptProjectsForRemote()
	assert.False(t, on(s), "no bots, nothing to keep working")

	s = newServer(t, true)
	s.adoptProjectsForRemote()
	assert.True(t, on(s))

	s = newServer(t, true)
	require.NoError(t, s.config.SetScenarioFlag(typ.ScenarioGlobal, constant.ExtensionProjects, false))
	s.adoptProjectsForRemote()
	assert.False(t, on(s), "a switch turned off stays off")
}
