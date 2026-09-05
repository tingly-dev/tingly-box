#!/usr/bin/env node

import { existsSync, mkdirSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { cacheDir } from "../shared/cachedir.js";
import { cleanupRetiredInstallDirs, cleanupStaleBinaryCaches } from "../shared/cleanup.js";
import { downloadAndExtractZip } from "../shared/download.js";
import { DEFAULT_ARGS, bundleSwitchHints, sourceArgs } from "../shared/entry.js";
import { execBinary } from "../shared/exec.js";
import { parseTransportVersion } from "../shared/transport.js";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

const { version: VERSION, remainingArgs } = parseTransportVersion();

const SOURCE_ARGS = sourceArgs("npx", "npm");

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

	// For the NPX package, we always use the configured branch or the specified version
	const branchName = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

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

	// If binary doesn't exist, download and extract ZIP
	if (!existsSync(binaryPath)) {
		await downloadAndExtractZip(downloadUrl, tinglyBinDir, {
			hints: bundleSwitchHints(branchName),
		});

		console.log(`✅ Downloaded and extracted to ${binaryPath}`);
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
