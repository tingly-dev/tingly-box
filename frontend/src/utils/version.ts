// Build versions reach the UI in two shapes: the running binary's own
// version is the git tag it was built from ("v1.261001.1", or "v1.261001.1-
// 27-g4e437c592" for a dev build), while the latest release comes from npm
// without the "v" ("1.261002.0"). Some carry "+<build metadata>". Callers
// used to prepend their own "v", which showed "vv1.261001.1".

const NUMERIC = /^\d/;

/** The version without its "v" or build metadata: "1.261001.1-27-g4e437c592". */
export function bareVersion(version: string): string {
    const v = version.split('+')[0];
    return v.startsWith('v') && NUMERIC.test(v.slice(1)) ? v.slice(1) : v;
}

/**
 * The version for display: exactly one leading "v" on a numbered version
 * ("v1.261001.1"), anything else ("dev", "Unknown") as is. Mirrors the CLI's
 * formatVersion (internal/command/server.go).
 */
export function displayVersion(version: string | null | undefined): string {
    if (!version) return 'Unknown';
    const bare = bareVersion(version);
    return NUMERIC.test(bare) ? `v${bare}` : bare;
}
