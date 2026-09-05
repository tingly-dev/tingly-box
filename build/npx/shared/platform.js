// Platform binary packages: `tingly-box` declares one optionalDependency per
// supported platform (tingly-box-linux-x64, …); npm installs only the one
// whose os/cpu match and skips the rest silently. Each carries the raw Go
// binary at bin/tingly-box[.exe], so an install needs nothing but the npm
// registry (mirrors included) — no GitHub download. The cli shim resolves it
// from here and falls back to the release download when it is missing (e.g.
// `--no-optional`, a registry mirror that hasn't synced the platform package
// yet, or a dev checkout). Rationale: .design/npm.md ("F. Platform packages").
//
// scripts/build-platform-packages.sh and the publish workflow read this map
// (via `node -e`) to build and wire the packages, so it is the single source
// of truth for names and the release-zip mapping.

import { createRequire } from "module";
import { existsSync } from "fs";
import { dirname, join } from "path";

// key: `${process.platform}-${process.arch}` → { name, zip }
// zip: the release asset name (release.yml) the package is built from.
export const PLATFORM_PACKAGES = {
	"linux-x64": { name: "tingly-box-linux-x64", zip: "tingly-box-linux-amd64.zip" },
	"linux-arm64": { name: "tingly-box-linux-arm64", zip: "tingly-box-linux-arm64.zip" },
	"darwin-x64": { name: "tingly-box-darwin-x64", zip: "tingly-box-macos-amd64.zip" },
	"darwin-arm64": { name: "tingly-box-darwin-arm64", zip: "tingly-box-macos-arm64.zip" },
	"win32-x64": { name: "tingly-box-win32-x64", zip: "tingly-box-windows-amd64.zip" },
};

export function platformPackageName() {
	const entry = PLATFORM_PACKAGES[`${process.platform}-${process.arch}`];
	return entry ? entry.name : null;
}

// Locate the installed platform package for this host. Resolution starts
// from `fromUrl` (the calling shim's import.meta.url), so it finds the
// package wherever npm put it: nested under the global install, hoisted in
// npx's cache, or a dev checkout's node_modules. Returns
// { name, version, binaryPath } or null when not installed / incomplete.
// Never throws — the caller treats null as "use the download path".
export function findPlatformPackage(fromUrl) {
	const name = platformPackageName();
	if (!name) return null;
	try {
		// Distinct identifier from the `require` the esbuild banner defines.
		const resolver = createRequire(fromUrl);
		const pkgJsonPath = resolver.resolve(`${name}/package.json`);
		const pkg = resolver(pkgJsonPath);
		const binaryName = "tingly-box" + (process.platform === "win32" ? ".exe" : "");
		const binaryPath = join(dirname(pkgJsonPath), "bin", binaryName);
		if (!existsSync(binaryPath)) return null;
		return { name, version: pkg.version, binaryPath };
	} catch {
		return null;
	}
}
