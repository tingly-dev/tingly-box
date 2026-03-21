package lark

import (
	"testing"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/feishu"
)

func TestMenuAdapterSupports(t *testing.T) {
	adapter := NewMenuAdapter()

	if !adapter.Supports(core.PlatformLark) {
		t.Errorf("Expected adapter to support Lark")
	}

	if adapter.Supports(core.PlatformFeishu) {
		t.Errorf("Expected adapter not to support Feishu (use feishu.NewMenuAdapter instead)")
	}

	if adapter.Supports(core.PlatformTelegram) {
		t.Errorf("Expected adapter not to support Telegram")
	}
}

func TestMenuAdapterGetDomain(t *testing.T) {
	adapter := NewMenuAdapter()

	if adapter.GetDomain() != feishu.DomainLark {
		t.Errorf("Expected domain '%s', got '%s'", feishu.DomainLark, adapter.GetDomain())
	}
}

func TestMenuAdapterDelegatesToFeishu(t *testing.T) {
	// Lark adapter should delegate to Feishu implementation
	adapter := NewMenuAdapter()

	if adapter.MenuAdapter == nil {
		t.Error("Expected MenuAdapter to be initialized")
	}
}
