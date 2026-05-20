#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, existsSync, renameSync, rmSync, statSync, writeFileSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { mkdir } from "fs/promises";
import unzipper from "unzipper";
import { homedir } from "os";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

const __dirname = dirname(fileURLToPath(import.meta.url));

function getPlatformInfo() {
	const platform = process.platform;
	const arch = process.arch;

	if (platform === "darwin") {
		return arch === "arm64" ? "macos-arm64" : "macos-amd64";
	} else if (platform === "linux") {
		if (arch === "x64") return "linux-amd64";
		if (arch === "arm64") return "linux-arm64";
		throw new Error(`Unsupported arch: ${arch}`);
	} else if (platform === "win32") {
		if (arch === "x64") return "windows-amd64";
		throw new Error(`Unsupported arch: ${arch}`);
	}
	throw new Error(`Unsupported platform: ${platform}`);
}

// Get cache directory for extracted binaries
function getCacheDir() {
	const baseDir = process.env.XDG_CACHE_HOME || join(homedir(), ".cache");
	const cacheDir = join(baseDir, "tingly-box-bundle", BINARY_RELEASE_BRANCH);
	return cacheDir;
}

// Extract binary from zip to cache directory
async function extractBinary(platformDir) {
	const zipFileName = `tingly-box-${platformDir}.zip`;
	const zipPath = join(__dirname, "zip", zipFileName);
	const cacheDir = getCacheDir();
	const targetPath = join(cacheDir, platformDir);

	// All platforms now use unified binary name "tingly-box"
	const binaryName = "tingly-box" + (process.platform === "win32" ? ".exe" : "");
	const cachedBinary = join(targetPath, binaryName);

	// Read the zip central directory to locate the binary entry and learn its
	// real uncompressed size — used both to validate the cache and the extraction.
	const directory = await unzipper.Open.file(zipPath);
	const entry = directory.files.find(
		(f) =>
			f.type !== "Directory" &&
			(f.path === binaryName || f.path.endsWith(`/${binaryName}`))
	);
	if (!entry) {
		throw new Error(`Binary "${binaryName}" not found inside ${zipFileName}`);
	}

	// Reuse the cache only when the size matches the zip entry. A bare
	// existence check would happily reuse a truncated file left behind by an
	// interrupted extraction (on Windows that surfaces as "This app can't run
	// on your PC"); a size mismatch triggers a clean re-extraction instead.
	if (existsSync(cachedBinary)) {
		try {
			if (statSync(cachedBinary).size === entry.uncompressedSize) {
				return cachedBinary;
			}
		} catch {
			// fall through to re-extract
		}
		console.warn(`⚠️  Cached binary is corrupted, re-extracting...`);
	}

	// Create cache directory
	await mkdir(targetPath, { recursive: true });

	console.log(`📦 Extracting ${zipFileName}...`);

	const content = await entry.buffer();
	if (content.length !== entry.uncompressedSize) {
		throw new Error(
			`Extraction produced ${content.length} bytes, expected ${entry.uncompressedSize}`
		);
	}

	// Write to a temp file, then rename into place. The final path only ever
	// holds a complete binary — a crash mid-write can no longer poison the cache.
	const tmpPath = join(targetPath, `.${binaryName}.tmp-${process.pid}`);
	try {
		writeFileSync(tmpPath, content);
		if (process.platform !== "win32") {
			chmodSync(tmpPath, 0o755);
		}
		try {
			renameSync(tmpPath, cachedBinary);
		} catch {
			// Windows can refuse to replace an existing (e.g. locked) file —
			// remove the stale one and retry once.
			rmSync(cachedBinary, { force: true });
			renameSync(tmpPath, cachedBinary);
		}
	} catch (e) {
		rmSync(tmpPath, { force: true });
		throw e;
	}

	console.log(`✅ Extracted to: ${cachedBinary}`);
	return cachedBinary;
}

// Default parameters to use when no arguments are provided
const DEFAULT_ARGS = [
	"restart",
	"--daemon",
];

// Global flag prepended to every invocation so the binary records that it was
// launched via npx-bundle. As a global flag it must come before the subcommand.
const SOURCE_ARGS = ["--source=npx-bundle"];

const args = process.argv.slice(2);
const baseArgs = args.length > 0 ? args : DEFAULT_ARGS;
const argsToUse = [...SOURCE_ARGS, ...baseArgs];

const platformDir = getPlatformInfo();

// Verify zip exists
const zipFileName = `tingly-box-${platformDir}.zip`;
const zipPath = join(__dirname, "zip", zipFileName);
if (!existsSync(zipPath)) {
	console.error(`❌ Zip file not found: ${zipPath}`);
	console.error(`This platform is not supported for current version.`);
	process.exit(1);
}

// Extract binary and get path
const binaryPath = await extractBinary(platformDir);

try {
	execFileSync(binaryPath, argsToUse, {
		stdio: "inherit",
		encoding: 'utf8'
	});
} catch (execError) {
	const errorCode = execError.code;
	const errorMessage = execError.message;
	const errorStatus = execError.status;
	const errorSignal = execError.signal;

	// Create comprehensive error output
	console.error(`\n❌ Tingly-Box execution failed`);
	console.error(`┌─ Error Details:`);
	console.error(`│  Message: ${errorMessage}`);

	if (errorCode) {
		console.error(`│  Code: ${errorCode}`);
		switch (errorCode) {
			case 'ENOENT':
				console.error(`│  └─ Binary not found at: ${binaryPath}`);
				break;
			case 'EACCES':
				console.error(`│  └─ Permission denied. Try: chmod +x "${binaryPath}"`);
				break;
			case 'ETXTBSY':
				console.error(`│  └─ Binary file is busy or being modified.`);
				break;
			default:
				console.error(`│  └─ System error occurred.`);
		}
	}

	if (errorStatus !== null && errorStatus !== undefined) {
		console.error(`│  Exit Code: ${errorStatus}`);
		console.error(`│  └─ The binary exited with non-zero status code.`);
	}

	if (errorSignal) {
		console.error(`│  Signal: ${errorSignal}`);
		console.error(`│  └─ The binary was terminated by a signal.`);
	}

	console.error(`└─ Binary Path: ${binaryPath}`);
	console.error(`   Platform: ${process.platform} (${process.arch})`);

	// Provide additional help for Linux
	if (process.platform === "linux") {
		console.error(`\n💡 Linux Troubleshooting:`);
		console.error(`   • Check if required libraries are installed`);
		console.error(`   • For missing dependencies: install required system packages`);
	}

	// Exit with the binary's exit code
	const exitCode = errorStatus !== undefined ? errorStatus : 1;
	process.exit(exitCode);
}
