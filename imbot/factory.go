package imbot

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform"
)

// CreateBot creates a bot instance based on the configuration
func CreateBot(config *core.Config) (core.Bot, error) {
	// Validate config
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Expand environment variables
	config.ExpandEnvVars()

	// Create bot using platform registry
	return platform.Create(config)
}

// CreateBots creates multiple bot instances
func CreateBots(configs []*core.Config) ([]core.Bot, error) {
	var bots []core.Bot
	var errs []error

	for _, config := range configs {
		bot, err := CreateBot(config)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create %s bot: %w", config.Platform, err))
			continue
		}
		bots = append(bots, bot)
	}

	if len(errs) > 0 {
		return bots, fmt.Errorf("created %d/%d bots with errors: %v", len(bots), len(configs), errs)
	}

	return bots, nil
}

// IsPlatformSupported checks if a platform is supported
func IsPlatformSupported(platformStr string) bool {
	return platform.IsSupported(Platform(platformStr))
}

// SupportedPlatforms returns a list of all supported platforms
func SupportedPlatforms() []string {
	ps := platform.SupportedPlatforms()
	platforms := make([]string, len(ps))
	for i, p := range ps {
		platforms[i] = string(p)
	}
	return platforms
}

// GetPlatformInfo returns information about a platform
func GetPlatformInfo(platform string) (*core.PlatformInfo, error) {
	p := Platform(platform)
	if !core.IsValidPlatform(platform) {
		return nil, fmt.Errorf("invalid platform: %s", platform)
	}
	return core.NewPlatformInfo(p, core.GetPlatformName(p)), nil
}
