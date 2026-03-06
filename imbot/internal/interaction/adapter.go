package interaction

import (
	"github.com/tingly-dev/tingly-box/imbot/internal/itx"
)

// Re-export Adapter interface from itx package
type Adapter = itx.Adapter
type BaseAdapter = itx.BaseAdapter

// NewBaseAdapter creates a new base adapter with the given capabilities
func NewBaseAdapter(supportsInteractions, canEditMessages bool) *BaseAdapter {
	return itx.NewBaseAdapter(supportsInteractions, canEditMessages)
}
