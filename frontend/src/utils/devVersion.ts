// Dev builds report their version as just "dev" (see internal/command's
// BuildVersion default), which is useless when you have several checkouts or
// branches in flight. The Vite config stamps the current commit into the
// bundle at build / dev-server start instead, and displayVersion() swaps it in
// for "dev". Nothing here runs in the browser at runtime: the browser can't
// reach git, so the value is baked in by `define` (see vite.config.ts).

/**
 * Seven hex digits is git's default short length (`git rev-parse --short`)
 * and unique for this repo's history — collisions need millions of commits.
 * Keep fewer than the full 40 so the footer stays readable.
 */
export const DEV_SHORT_SHA_LENGTH = 7;

/**
 * Build the dev version label: "dev-<short sha>", plus "-dirty" when the
 * working tree has changes so a local uncommitted edit is distinguishable
 * from the commit it sits on. No sha (no git / not a checkout) falls back to
 * plain "dev", exactly what the backend reports.
 */
export function buildDevVersion(shortSha: string | null | undefined, dirty: boolean): string {
    if (!shortSha) return 'dev';
    return dirty ? `dev-${shortSha}-dirty` : `dev-${shortSha}`;
}

/**
 * Read the working tree's short sha and dirty state through `run` (a command
 * runner supplied by the caller, so this stays node-free and testable).
 * Any git failure — not a checkout, git missing — falls back to "dev" rather
 * than breaking the build.
 */
export function readGitDevVersion(run: (command: string) => string): string {
    try {
        const shortSha = run(`git rev-parse --short=${DEV_SHORT_SHA_LENGTH} HEAD`).trim();
        // Guard against git printing an error to stdout instead of throwing.
        if (!/^[0-9a-f]{4,40}$/i.test(shortSha)) return buildDevVersion(null, false);
        // --porcelain is repository-wide; ignored files (dist, node_modules)
        // don't appear, so only real edits/fixtures flip this to "-dirty".
        const dirty = run('git status --porcelain').trim().length > 0;
        return buildDevVersion(shortSha, dirty);
    } catch {
        return buildDevVersion(null, false);
    }
}
