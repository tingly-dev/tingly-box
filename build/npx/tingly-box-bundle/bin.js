#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, existsSync, readdirSync, statSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { createReadStream } from "fs";
import { mkdir } from "fs/promises";
import { pipeline } from "stream/promises";
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

// Recursively find binary in directory (handles nested directory structures in zip)
function findBinary(dir, binaryName) {
	const entries = readdirSync(dir);
	for (const entry of entries) {
		const fullPath = join(dir, entry);
		try {
			const stat = statSync(fullPath);
			if (stat.isFile() && entry === binaryName) {
				return fullPath;
			}
			if (stat.isDirectory()) {
				const found = findBinary(fullPath, binaryName);
				if (found) return found;
			}
		} catch {
			continue;
		}
	}
	return null;
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

	// Check if binary already exists in cache and has executable permission
	if (existsSync(cachedBinary)) {
		return cachedBinary;
	}

	// Create cache directory
	await mkdir(targetPath, { recursive: true });

	console.log(`📦 Extracting ${zipFileName}...`);

	// Extract zip file - the zip contains the binary at root level
	await pipeline(
		createReadStream(zipPath),
		unzipper.Extract({ path: targetPath })
	);

	// Find the actual binary (handles cases where zip has nested structure)
	let actualBinaryPath = cachedBinary;
	if (!existsSync(cachedBinary)) {
		const found = findBinary(targetPath, binaryName);
		if (found) {
			actualBinaryPath = found;
		} else {
			throw new Error(`Binary "${binaryName}" not found after extraction`);
		}
	}

	// Set executable permission on Unix systems
	if (process.platform !== "win32") {
		try {
			chmodSync(actualBinaryPath, 0o755);
		} catch (e) {
			console.warn(`⚠️  Failed to set executable permission: ${e.message}`);
		}
	}

	console.log(`✅ Extracted to: ${actualBinaryPath}`);
	return actualBinaryPath;
}

// Default parameters to use when no arguments are provided
const DEFAULT_ARGS = [
	"start",
	"--daemon",
];

const args = process.argv.slice(2);
const argsToUse = args.length > 0 ? args : DEFAULT_ARGS;

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
