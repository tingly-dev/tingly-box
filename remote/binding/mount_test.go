package binding

import "testing"

func TestSetScenarioEnabled(t *testing.T) {
	// Append to empty.
	out, err := SetScenarioEnabled("", RemoteAgentScenario, true)
	if err != nil {
		t.Fatal(err)
	}
	if !ScenarioMounted(out, RemoteAgentScenario) {
		t.Fatalf("expected mounted after enabling, got %q", out)
	}

	// Toggle off updates in place and reads back as not mounted.
	out, err = SetScenarioEnabled(out, RemoteAgentScenario, false)
	if err != nil {
		t.Fatal(err)
	}
	if ScenarioMounted(out, RemoteAgentScenario) {
		t.Fatalf("expected not mounted after disabling, got %q", out)
	}

	// Preserves other bindings and their fields.
	base := `[{"name":"claude_code","chat_id":"c1","events":["Stop"]}]`
	out, err = SetScenarioEnabled(base, RemoteAgentScenario, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parse(out)
	if err != nil {
		t.Fatal(err)
	}
	var sawCC, sawRA bool
	for _, b := range got {
		if b.Name == "claude_code" {
			sawCC = true
			if b.ChatID != "c1" {
				t.Fatalf("claude_code chat_id lost: %+v", b)
			}
		}
		if b.Name == RemoteAgentScenario {
			sawRA = true
		}
	}
	if !sawCC || !sawRA {
		t.Fatalf("expected both bindings preserved, got %+v", got)
	}

	// Malformed input surfaces an error and returns the original.
	if _, err := SetScenarioEnabled(`{bad`, RemoteAgentScenario, true); err == nil {
		t.Fatal("expected error on malformed scenarios")
	}
}

func TestOutboundScenarioMounted(t *testing.T) {
	cases := []struct {
		name      string
		scenarios string
		want      bool
	}{
		{"empty is NOT mounted (nothing to route)", ``, false},
		{"remote_agent alone is NOT an outbound mount",
			`[{"name":"remote_agent","enabled":true}]`, false},
		{"outbound binding present is mounted",
			`[{"name":"claude_code","chat_id":"c"}]`, true},
		{"outbound binding enabled true is mounted",
			`[{"name":"claude_code","chat_id":"c","enabled":true}]`, true},
		{"outbound binding disabled is NOT mounted",
			`[{"name":"claude_code","chat_id":"c","enabled":false}]`, false},
		{"one of several outbound bindings on is mounted",
			`[{"name":"claude_code","enabled":false},{"name":"deploy","chat_id":"c"}]`, true},
		{"remote_agent off + outbound on is mounted",
			`[{"name":"remote_agent","enabled":false},{"name":"claude_code","chat_id":"c"}]`, true},
		{"malformed is NOT mounted", `{not json`, false},
		{"nameless binding does not count", `[{"chat_id":"c"}]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OutboundScenarioMounted(c.scenarios); got != c.want {
				t.Fatalf("OutboundScenarioMounted(%q) = %v, want %v", c.scenarios, got, c.want)
			}
		})
	}
}

func TestScenarioMounted(t *testing.T) {
	cases := []struct {
		name      string
		scenarios string
		want      bool
	}{
		{"empty is mounted (legacy default on)", ``, true},
		{"absent binding is mounted (legacy default on)",
			`[{"name":"claude_code","chat_id":"c"}]`, true},
		{"explicit present without enabled is mounted",
			`[{"name":"remote_agent"}]`, true},
		{"explicit enabled true is mounted",
			`[{"name":"remote_agent","enabled":true}]`, true},
		{"explicit enabled false is NOT mounted",
			`[{"name":"remote_agent","enabled":false}]`, false},
		{"malformed is mounted (never silently offline)",
			`{not json`, true},
		{"other scenario off does not affect remote_agent",
			`[{"name":"claude_code","enabled":false}]`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ScenarioMounted(c.scenarios, RemoteAgentScenario); got != c.want {
				t.Fatalf("ScenarioMounted(%q) = %v, want %v", c.scenarios, got, c.want)
			}
		})
	}
}
