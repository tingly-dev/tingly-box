#!/usr/bin/env node

import { execFileSync, execSync } from "child_process";
import { chmodSync, existsSync } from "fs";
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

// Verify binary exists
if (!existsSync(binaryPath)) {
	console.error(`❌ Binary not found: ${binaryPath}`);
	console.error(`This should not happen with the bundled package.`);
	console.error(`Please reinstall: npm install -g tingly-box-bundle`);
	process.exit(1);
}

// Make sure the binary is executable on Unix systems
if (process.platform !== "win32") {
	try {
		chmodSync(binaryPath, 0o755);
	} catch (error) {
		console.error(`⚠️  Warning: Could not set executable permission: ${error.message}`);
	}
}

// Print binary location for auditability
console.log(`🔍 Binary: ${binaryPath}`);

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
