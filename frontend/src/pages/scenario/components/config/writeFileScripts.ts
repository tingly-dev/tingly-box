export interface WriteFileScriptsOpts {
    /** PowerShell variable holding the target directory, e.g. `$configDir`. */
    dirVar: string;
    /** PowerShell variable holding the target file path, e.g. `$configPath`. */
    fileVar: string;
    /** PowerShell statement resolving the directory into `dirVar`. */
    dirSetupWindows: string;
    /** PowerShell statement resolving the file path into `fileVar`. */
    fileSetupWindows: string;
    /** Bash statements preparing the target directory (before the `cat`). */
    dirSetupUnix: string;
    /** Bash path expression for the target file, e.g. `~/.codex/config.toml`. */
    fileUnix: string;
    /** File body spliced verbatim into both here-docs. */
    content: string;
}

// Builders for the PowerShell / bash here-doc snippets that write one config
// file by hand ("Windows" / "Linux/macOS" tabs of the manual setup sections).
// Only the directory resolution differs per tool (Codex: ~/.codex, Dsh:
// $DSH_HOME with a default fallback); the boilerplate around it is identical.
export const writeFileScripts = (opts: WriteFileScriptsOpts): { windows: string; unix: string } => {
    const { dirVar, fileVar, dirSetupWindows, fileSetupWindows, dirSetupUnix, fileUnix, content } = opts;
    return {
        windows: [
            dirSetupWindows,
            fileSetupWindows,
            '',
            `New-Item -ItemType Directory -Force -Path ${dirVar} | Out-Null`,
            '',
            `@'`,
            content,
            `'@ | Set-Content -Path ${fileVar}`,
        ].join('\n'),
        unix: [
            dirSetupUnix,
            '',
            `cat > ${fileUnix} <<'EOF'`,
            content,
            'EOF',
        ].join('\n'),
    };
};
