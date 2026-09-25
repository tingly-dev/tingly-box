package protocoltest

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestGoogleTargetAnswersEverySource pins that each client protocol that can
// be routed to a Google-style provider gets a real answer back — never an
// empty 200.
func TestGoogleTargetAnswersEverySource(t *testing.T) {
	t.Parallel()

	for _, source := range []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat} {
		for _, streaming := range []bool{false, true} {
			source, streaming := source, streaming
			t.Run(fmt.Sprintf("%s/stream=%v", source, streaming), func(t *testing.T) {
				t.Parallel()

				env := NewTestEnv(t)
				scenario := TextScenario()
				env.SetupRoute(source, protocol.TypeGoogle, scenario)
				model := env.findRouteModel(source, protocol.TypeGoogle, scenario.Name)
				path, body := buildRequest(source, model, streaming)

				status, raw := sendRaw(t, env, path, body)
				if status != http.StatusOK {
					t.Fatalf("status = %d: %s", status, raw)
				}
				if !strings.Contains(raw, "Paris") {
					t.Fatalf("response carries no answer:\n%q", raw)
				}
			})
		}
	}
}
