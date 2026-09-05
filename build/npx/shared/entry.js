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

// Next step when the GitHub release download is unreachable: the bundle
// package carries the binaries inside its npm tarball, so it only needs the
// npm registry (mirrors included). All shim packages own the same
// `tingly-box`/`tb` bin names, and npm refuses to relink a global bin owned
// by another package (EEXIST) before any package script runs — so switching
// a global install means uninstalling the old package first. `--force` is
// not an alternative: the old package stays installed and a later
// `npm uninstall -g` of it deletes the bins the bundle now uses.
// Rationale and verification: .design/npm.md ("Bin-name collision").
export function bundleSwitchHints(tag) {
	const ver = tag === "latest" ? "latest" : tag.replace(/^v/, "");
	if (IS_NPX) {
		return [
			`• Use the bundle package instead (binaries built-in, same commands):`,
			`    npx -y tingly-box-bundle@${ver}`,
		];
	}
	return [
		`• Switch to the bundle package (binaries built-in, same tingly-box / tb commands).`,
		`  Uninstall first — both packages own the same bin names, so npm refuses`,
		`  a side-by-side install (EEXIST):`,
		`    npm uninstall -g tingly-box`,
		`    npm install -g tingly-box-bundle@${ver}`,
	];
}
