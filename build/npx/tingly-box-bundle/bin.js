#!/usr/bin/env node

import { execFileSync } from "child_process";
import { join, dirname } from "path";
import { fileURLToPath } from "url";

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

const platformDir = getPlatformInfo();
const binaryName = "tingly-box" + (process.platform === "win32" ? ".exe" : "");
const binaryPath = join(__dirname, "bin", platformDir, binaryName);

// Default parameters to use when no arguments are provided
const DEFAULT_ARGS = [
	"start",
	"--daemon",
];

const args = process.argv.slice(2);
const argsToUse = args.length > 0 ? args : DEFAULT_ARGS;

try {
	execFileSync(binaryPath, argsToUse, {
		stdio: "inherit",
		encoding: 'utf8'
	});
} catch (error) {
	// Exit with the binary's exit code
	const exitCode = error.status !== undefined ? error.status : 1;
	process.exit(exitCode);
}
