#!/usr/bin/env node

import { chmodSync, copyFileSync, existsSync, mkdirSync, renameSync, rmSync } from "fs";
import { createRequire } from "module";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { cacheDir } from "../shared/cachedir.js";
import { cleanupRetiredInstallDirs, cleanupStaleBinaryCaches } from "../shared/cleanup.js";
import { downloadAndExtractZip } from "../shared/download.js";
import { DEFAULT_ARGS, downloadFailureHints, sourceArgs } from "../shared/entry.js";
import { execBinary } from "../shared/exec.js";
import { findPlatformPackage, platformPackageName } from "../shared/platform.js";
import { parseTransportVersion } from "../shared/transport.js";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

const { version: VERSION, remainingArgs } = parseTransportVersion();

const SOURCE_ARGS = sourceArgs("npx", "npm");

// This shim's own npm version; the platform package must carry the same one.
const OWN_VERSION = createRequire(import.meta.url)("./package.json").version;

// Where the binary comes from for this launch. An explicit
// --transport-version always means the release download of that tag. The
// default launch prefers the platform package npm installed next to this
// shim (no network, works through registry mirrors); it is used only when
// its version equals the shim's so a partial upgrade can't pair an old
// binary with a new shim. Otherwise: download the baked release tag.
function resolveBinarySource() {
	if (VERSION === "latest") {
		const local = findPlatformPackage(import.meta.url);
		if (local && local.version === OWN_VERSION) {
			return { kind: "package", tag: `v${local.version}`, local };
		}
		if (local) {
			console.warn(`⚠️  ${local.name}@${local.version} does not match tingly-box@${OWN_VERSION}, downloading the release instead`);
		}
	}
	return { kind: "download", tag: VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION };
}

// Copy the platform package's binary into the versioned cache dir, writing a
// temp file and renaming so the final path only ever holds a complete
// binary. The cache copy — rather than exec'ing inside node_modules — keeps
// the invariants the download path already has: a running daemon keeps its
// inode while `npm install -g` retires the package dir (on Windows npm
// could not even remove a locked exe), and the stale-cache sweep, shortcut
// relaunch and `--transport-version` all keep one layout.
function installFromPackage(local, binaryPath) {
	console.log(`📦 Installing binary from ${local.name}@${local.version}...`);
	const tmpPath = `${binaryPath}.tmp-${process.pid}`;
	try {
		copyFileSync(local.binaryPath, tmpPath);
		if (process.platform !== "win32") {
			chmodSync(tmpPath, 0o755);
		}
		try {
			renameSync(tmpPath, binaryPath);
		} catch {
			rmSync(binaryPath, { force: true });
			renameSync(tmpPath, binaryPath);
		}
	} catch (e) {
		rmSync(tmpPath, { force: true });
		console.error(`❌ Failed to install binary from ${local.name}: ${e.message}`);
		process.exit(1);
	}
}

async function getPlatformArchAndBinary() {
	const platform = process.platform;
	const arch = process.arch;

	let platformDir;
	let archDir;
	let binaryName;
	binaryName = "tingly-box";
	let suffix = ""

	if (platform === "darwin") {
		platformDir = "macos";
		if (arch === "arm64") archDir = "arm64";
		else archDir = "amd64";
	} else if (platform === "linux") {
		platformDir = "linux";
		if (arch === "x64") archDir = "amd64";
		else if (arch === "ia32") archDir = "386";
		else archDir = arch; // fallback
	} else if (platform === "win32") {
		platformDir = "windows";
		if (arch === "x64") archDir = "amd64";
		else if (arch === "ia32") archDir = "386";
		else archDir = arch; // fallback
		suffix = ".exe";
	} else {
		console.error(`Unsupported platform/arch: ${platform}/${arch}`);
		process.exit(1);
	}

	return { platformDir, archDir, binaryName, suffix };
}

(async () => {
	cleanupRetiredInstallDirs(dirname(fileURLToPath(import.meta.url)));

	const platformInfo = await getPlatformArchAndBinary();
	const { platformDir, archDir, binaryName, suffix } = platformInfo;

	const source = resolveBinarySource();
	// Cache dir is keyed by the binary's release tag, whichever way it arrives.
	const branchName = source.tag;

	// Build ZIP download URL
	const zipFileName = `${binaryName}-${platformDir}-${archDir}.zip`;
	const downloadUrl = `${BASE_URL}/${branchName}/${zipFileName}`;

	// Use branch name for caching
	const cacheRoot = join(cacheDir(), "tingly-box");
	const tinglyBinDir = join(cacheRoot, branchName, "bin");

	// Create the binary directory
	try {
		if (!existsSync(tinglyBinDir)) {
			mkdirSync(tinglyBinDir, { recursive: true });
		}
	} catch (mkdirError) {
		console.error(`❌ Failed to create directory ${tinglyBinDir}:`, mkdirError.message);
		process.exit(1);
	}

	// The extracted binary path (now all platforms use unified name "tingly-box")
	const binaryPath = join(tinglyBinDir, `${binaryName}${suffix}`);

	// If binary doesn't exist, install it from the platform package or
	// download and extract the release ZIP
	if (!existsSync(binaryPath)) {
		if (source.kind === "package") {
			installFromPackage(source.local, binaryPath);
			console.log(`✅ Installed to ${binaryPath}`);
		} else {
			await downloadAndExtractZip(downloadUrl, tinglyBinDir, {
				// A default launch only reaches the download when the platform
				// package is missing; an explicit --transport-version asked for it.
				hints: VERSION === "latest" ? downloadFailureHints(platformPackageName(), OWN_VERSION) : [],
			});
			console.log(`✅ Downloaded and extracted to ${binaryPath}`);
		}
	}

	// The binary for this tag is in place — old tag dirs are now safe to GC.
	cleanupStaleBinaryCaches(cacheRoot, branchName);

	console.log(`🔍 Executing binary: ${binaryPath}`);

	// Use default args if no arguments provided; always prepend the source
	// flag so any npx invocation records its launch source.
	const baseArgs = remainingArgs.length > 0 ? remainingArgs : DEFAULT_ARGS;
	execBinary(binaryPath, [...SOURCE_ARGS, ...baseArgs], {
		retryCmd: `npx tingly-box ${remainingArgs.join(' ')}`,
		cacheRoot,
	});
})();
