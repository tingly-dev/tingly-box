package imbot

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform"
)

// TestPlatformConfigDisplayNamesDerived asserts every settings-UI entry takes
// its display name from the single source of truth in core, and that every
// configured platform is a known platform there.
func TestPlatformConfigDisplayNamesDerived(t *testing.T) {
	for id, cfg := range PlatformConfigs {
		if !core.IsValidPlatform(id) {
			t.Errorf("PlatformConfigs has entry %q that is not a valid core platform", id)
		}
		want := core.GetPlatformName(core.Platform(id))
		if cfg.DisplayName != want {
			t.Errorf("PlatformConfigs[%q].DisplayName = %q, want %q (derived from core)", id, cfg.DisplayName, want)
		}
		if cfg.DisplayName == "" {
			t.Errorf("PlatformConfigs[%q] has empty DisplayName", id)
		}
		if cfg.Platform != id {
			t.Errorf("PlatformConfigs[%q].Platform = %q, want %q", id, cfg.Platform, id)
		}
	}
}

// TestConfigurablePlatformsMatchRegistry is the key anti-drift guarantee: the
// set of platforms with a settings/auth-config form must equal the set of
// platforms the registry can actually instantiate. If someone adds a bot
// creator but forgets its config form (or vice-versa), this fails.
func TestConfigurablePlatformsMatchRegistry(t *testing.T) {
	configured := make(map[string]bool, len(PlatformConfigs))
	for id := range PlatformConfigs {
		configured[id] = true
	}

	creatable := make(map[string]bool)
	for _, p := range platform.SupportedPlatforms() {
		creatable[string(p)] = true
	}

	for id := range configured {
		if !creatable[id] {
			t.Errorf("platform %q has a config form but no registry creator", id)
		}
	}
	for id := range creatable {
		if !configured[id] {
			t.Errorf("platform %q has a registry creator but no config form", id)
		}
	}
}

// TestGetAllPlatformsPopulated ensures the accessor used by the settings API
// returns fully-populated entries (display names filled in by init).
func TestGetAllPlatformsPopulated(t *testing.T) {
	all := GetAllPlatforms()
	if len(all) != len(PlatformConfigs) {
		t.Fatalf("GetAllPlatforms returned %d, want %d", len(all), len(PlatformConfigs))
	}
	for _, cfg := range all {
		if cfg.DisplayName == "" {
			t.Errorf("GetAllPlatforms returned %q with empty DisplayName", cfg.Platform)
		}
	}
}
