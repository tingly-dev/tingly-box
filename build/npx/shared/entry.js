// Entry semantics shared by the cli and bundle shims.
// npx / `npm exec` sets npm_command=exec; a bin launched directly (e.g. from
// `npm install -g`) does not. Rationale: .design/cli-entry-semantics.md.

export const IS_NPX = process.env.npm_command === "exec";

// Bare npx invocation = "run it now" (restart into the background, -y being
// the consent a bare `restart` would otherwise prompt for); a bare installed
// bin shows help.
export const DEFAULT_ARGS = IS_NPX ? ["restart", "--daemon", "-y"] : ["--help"];

// Records how this process was launched; also decides which npm package a
// shortcut relaunches. As a global flag it must come before the subcommand.
export function sourceArgs(npxSource, npmSource) {
	return [IS_NPX ? `--source=${npxSource}` : `--source=${npmSource}`];
}

// Next steps when the GitHub release download fails. The normal install
// never needs it: the platform package (shared/platform.js) ships the binary
// through the npm registry. Reaching the download at all means that package
// is missing — so the fix is to reinstall so npm fetches it (a mirror
// registry works too), and only then to retry or proxy the download.
export function downloadFailureHints(platformPackage, version) {
	if (!platformPackage) return [];
	const spec = `tingly-box@${version}`;
	return [
		`• The binary normally ships in the ${platformPackage} package, which is not installed here.`,
		IS_NPX
			? `  Re-run with optional dependencies enabled so npm fetches it`
			: `  Reinstall with optional dependencies enabled so npm fetches it`,
		`  (an npm mirror registry works too — no GitHub access needed):`,
		IS_NPX ? `    npx -y ${spec}` : `    npm install -g ${spec}`,
	];
}
