package agent

import "embed"

// scriptAssets embeds the scripts InstallStatusLineScript writes to disk.
// This duplicates internal/script/tingly-statusline.sh (embedded separately
// by internal/assets.go's ScriptAssets for internal/server/config's own
// copy) so ai/agent does not need to import the parent repo's internal
// package — see .sdlc/docs/ai-module-decoupling-refactor-20260803.spec.md.
// Only the status line script is needed here: InstallNotifyScript and
// InstallIMHookScript are not part of ai/agent's Apply path and remain
// internal/server/config-only.
//
//go:embed script/tingly-statusline.sh
var scriptAssets embed.FS
