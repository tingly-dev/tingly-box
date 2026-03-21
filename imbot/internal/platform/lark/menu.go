package lark

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/menu"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/feishu"
)

// MenuAdapter implements menu support for Lark platform.
// Lark uses the same implementation as Feishu, just with a different domain.
type MenuAdapter struct {
	*feishu.MenuAdapter
}

// NewMenuAdapter creates a new Lark menu adapter
func NewMenuAdapter() *MenuAdapter {
	return &MenuAdapter{
		MenuAdapter: feishu.NewMenuAdapterWithDomain(feishu.DomainLark),
	}
}

// Supports checks if this adapter supports Lark
func (a *MenuAdapter) Supports(platform core.Platform) bool {
	return platform == core.PlatformLark
}

// ShowMenu displays a menu on Lark (delegates to Feishu implementation)
func (a *MenuAdapter) ShowMenu(ctx context.Context, bot core.Bot, menuCtx *menu.MenuContext, m *menu.Menu) (*menu.MenuResult, error) {
	// Update platform to Lark for context
	newMenuCtx := menu.NewMenuContext(menuCtx.ChatID, core.PlatformLark)
	newMenuCtx.MessageID = menuCtx.MessageID
	for k, v := range menuCtx.Metadata {
		newMenuCtx.Metadata[k] = v
	}

	return a.MenuAdapter.ShowMenu(ctx, bot, newMenuCtx, m)
}

// GetDomain returns the Lark domain
func (a *MenuAdapter) GetDomain() feishu.Domain {
	return feishu.DomainLark
}
