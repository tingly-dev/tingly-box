#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, existsSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { createReadStream } from "fs";
import { mkdir } from "fs/promises";
import { pipeline } from "stream/promises";
import unzipper from "unzipper";
import { homedir } from "os";

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
	const cacheDir = join(baseDir, "tingly-box-bundle");
	return cacheDir;
}

// Extract binary from zip to cache directory
async function extractBinary(platformDir) {
	const zipFileName = `tingly-box-${platformDir}.zip`;
	const zipPath = join(__dirname, "zip", zipFileName);
	const cacheDir = getCacheDir();
	const targetPath = join(cacheDir, platformDir);

	// Check if binary already exists in cache
	const binaryName = "tingly-box" + (process.platform === "win32" ? ".exe" : "");
	const cachedBinary = join(targetPath, binaryName);
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

	// Set executable permission on Unix systems
	if (process.platform !== "win32") {
		chmodSync(cachedBinary, 0o755);
	}

	console.log(`✅ Extracted to: ${cachedBinary}`);
	return cachedBinary;
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
	console.error(`This should not happen with the bundled package.`);
	console.error(`Please reinstall: npm install -g tingly-box-bundle`);
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
				console.error(`│     Please reinstall: npm install -g tingly-box-bundle`);
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
