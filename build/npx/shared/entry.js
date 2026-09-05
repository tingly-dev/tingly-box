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

// Next steps when the GitHub release download fails on a default launch.
// Reaching the download at all means the platform package
// (shared/platform.js) that normally ships the binary is missing or at the
// wrong version. The fix is a fresh global install: npm resolves the
// optional dependency again (a mirror registry works too). Under npx the
// advice is the same — npx reuses its cached tree for a repeated spec and
// would never refetch the package.
export function downloadFailureHints(platformPackage, version, installedVersion) {
	if (!platformPackage) return [];
	return [
		installedVersion
			? `• ${platformPackage}@${installedVersion} is installed but tingly-box@${version} needs the same version.`
			: `• The binary normally ships in the ${platformPackage} package, which is not installed here.`,
		`  Install fresh so npm fetches it (an npm mirror registry works too — no GitHub access needed):`,
		`    npm install -g tingly-box@${version}`,
	];
}
