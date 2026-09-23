/**
 * IDE Sources configuration
 * Maps IDE source keys to their display names and icons
 */

export const IDE_SOURCES = {
    claude_code: { name: 'Claude Code', icon: '🎨' },
    opencode: { name: 'OpenCode', icon: '💻' },
    vscode: { name: 'VS Code', icon: '💡' },
    cursor: { name: 'Cursor', icon: '🎯' },
    codex: { name: 'Codex', icon: '📜' },
    antigravity: { name: 'Antigravity', icon: '🔄' },
    amp: { name: 'Amp', icon: '⚡' },
    kilo_code: { name: 'Kilo Code', icon: '🪜' },
    roo_code: { name: 'Roo Code', icon: '🦘' },
    goose: { name: 'Goose', icon: '🪿' },
    gemini_cli: { name: 'Gemini CLI', icon: '💎' },
    github_copilot: { name: 'GitHub Copilot', icon: '🐙' },
    clawdbot: { name: 'Clawdbot', icon: '🦞' },
    droid: { name: 'Droid', icon: '🤖' },
    windsurf: { name: 'Windsurf', icon: '🌊' },
    custom: { name: 'Custom', icon: '📂' },
} as const;

export type IDESourceKey = keyof typeof IDE_SOURCES;

export const getIdeSourceLabel = (source: string): string => {
    return IDE_SOURCES[source as IDESourceKey]?.name || source;
};
