#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, createWriteStream, existsSync, fsyncSync, mkdirSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { Readable } from "stream";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download/";

// GitHub API endpoint for getting latest release info
const LATEST_RELEASE_API_URL = "https://github.com/tingly-dev/tingly-box/releases/download/";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

// Parse transport version from command line arguments
function parseTransportVersion() {
	const args = process.argv.slice(2);
	let transportVersion = "latest"; // Default to latest

	// Find --transport-version argument
	const versionArgIndex = args.findIndex((arg) => arg.startsWith("--transport-version"));

	if (versionArgIndex !== -1) {
		const versionArg = args[versionArgIndex];

		if (versionArg.includes("=")) {
			// Format: --transport-version=v1.2.3
			transportVersion = versionArg.split("=")[1];
		} else if (versionArgIndex + 1 < args.length) {
			// Format: --transport-version v1.2.3
			transportVersion = args[versionArgIndex + 1];
		}

		// Remove the transport-version arguments from args array so they don't get passed to the binary
		if (versionArg.includes("=")) {
			args.splice(versionArgIndex, 1);
		} else {
			args.splice(versionArgIndex, 2);
		}
	}

	return { version: validateTransportVersion(transportVersion), remainingArgs: args };
}

// Validate transport version format
function validateTransportVersion(version) {
	if (version === "latest") {
		return version;
	}

	// Check if version matches v{x.x.x} format
	const versionRegex = /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
	if (versionRegex.test(version)) {
		return version;
	}

	console.error(`Invalid transport version format: ${version}`);
	console.error(`Transport version must be either "latest", "v1.2.3", or "v1.2.3-prerelease1"`);
	process.exit(1);
}

const { version: VERSION, remainingArgs } = parseTransportVersion();

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

async function downloadBinary(url, dest) {
	// console.log(`🔄 Downloading binary from ${url}...`);

	// Fetch with redirect following
	const res = await fetch(url, {
		redirect: 'follow', // Automatically follow redirects
		headers: {
			'User-Agent': 'tingly-box-npx'
		}
	});

	if (!res.ok) {
		console.error(`❌ Download failed: ${res.status} ${res.statusText}`);
		process.exit(1);
	}

	const contentLength = res.headers.get("content-length");
	const totalSize = contentLength ? parseInt(contentLength, 10) : null;
	let downloadedSize = 0;

	const fileStream = createWriteStream(dest, { flags: "w" });
	await new Promise((resolve, reject) => {
		try {
			// Convert the fetch response body to a Node.js readable stream
			const nodeStream = Readable.fromWeb(res.body);

			// Add progress tracking
			nodeStream.on("data", (chunk) => {
				downloadedSize += chunk.length;
				if (totalSize) {
					const progress = ((downloadedSize / totalSize) * 100).toFixed(1);
					process.stdout.write(`\r⏱️ Downloading Binary: ${progress}% (${formatBytes(downloadedSize)}/${formatBytes(totalSize)})`);
				} else {
					process.stdout.write(`\r⏱️ Downloaded: ${formatBytes(downloadedSize)}`);
				}
			});

			nodeStream.pipe(fileStream);
			fileStream.on("finish", () => {
				process.stdout.write("\n");

				// Ensure file is fully written to disk
				try {
					fsyncSync(fileStream.fd);
				} catch (syncError) {
					// fsync might fail on some systems, ignore
				}

				resolve();
			});
			fileStream.on("error", reject);
			nodeStream.on("error", reject);
		} catch (error) {
			reject(error);
		}
	});

	chmodSync(dest, 0o755);
}

async function downloadAndExtractZip(url, extractDir, binaryName) {
	console.log(`🔄 Downloading ZIP from ${url}...`);

	// Fetch with redirect following
	const res = await fetch(url, {
		redirect: 'follow',
		headers: {
			'User-Agent': 'tingly-box-npx'
		}
	});

	if (!res.ok) {
		console.error(`❌ Download failed: ${res.status} ${res.statusText}`);
		process.exit(1);
	}

	// Create a temporary file for the ZIP
	const zipPath = join(tmpdir(), `tingly-box-${Date.now()}.zip`);
	const fileStream = createWriteStream(zipPath, { flags: "w" });

	const contentLength = res.headers.get("content-length");
	const totalSize = contentLength ? parseInt(contentLength, 10) : null;
	let downloadedSize = 0;

	await new Promise((resolve, reject) => {
		try {
			const nodeStream = Readable.fromWeb(res.body);

			nodeStream.on("data", (chunk) => {
				downloadedSize += chunk.length;
				if (totalSize) {
					const progress = ((downloadedSize / totalSize) * 100).toFixed(1);
					process.stdout.write(`\r⏱️ Downloading ZIP: ${progress}% (${formatBytes(downloadedSize)}/${formatBytes(totalSize)})`);
				} else {
					process.stdout.write(`\r⏱️ Downloaded: ${formatBytes(downloadedSize)}`);
				}
			});

			nodeStream.pipe(fileStream);
			fileStream.on("finish", () => {
				process.stdout.write("\n");
				resolve();
			});
			fileStream.on("error", reject);
			nodeStream.on("error", reject);
		} catch (error) {
			reject(error);
		}
	});

	// Extract the ZIP file using system unzip command
	try {
		console.log(`📦 Extracting ZIP...`);
		execFileSync("unzip", ["-q", "-o", zipPath, "-d", extractDir]);
		console.log(`✅ Extracted ZIP to ${extractDir}`);
	} catch (error) {
		console.error(`❌ Failed to extract ZIP: ${error.message}`);
		// Fallback: try using Python to extract
		try {
			execFileSync("python3", ["-m", "zipfile", "-e", zipPath, extractDir]);
			console.log(`✅ Extracted ZIP using Python`);
		} catch (pythonError) {
			console.error(`❌ Failed to extract ZIP with Python too: ${pythonError.message}`);
			process.exit(1);
		}
	}

	// Clean up the ZIP file
	try {
		execFileSync("rm", ["-f", zipPath]);
	} catch (error) {
		// Ignore cleanup errors
	}
}

// Returns the os cache directory path for storing binaries
// Linux: $XDG_CACHE_HOME or ~/.cache
// macOS: ~/Library/Caches
// Windows: %LOCALAPPDATA% or %USERPROFILE%\AppData\Local
function cacheDir() {
	if (process.platform === "linux") {
		return process.env.XDG_CACHE_HOME || join(process.env.HOME || "", ".cache");
	}
	if (process.platform === "darwin") {
		return join(process.env.HOME || "", "Library", "Caches");
	}
	if (process.platform === "win32") {
		return process.env.LOCALAPPDATA || join(process.env.USERPROFILE || "", "AppData", "Local");
	}
	console.error(`Unsupported platform/arch: ${process.platform}/${process.arch}`);
	process.exit(1);
}

// gets the latest version number for transport
async function getLatestVersion() {
    const releaseUrl = LATEST_RELEASE_API_URL;
    const res = await fetch(releaseUrl);
    if (!res.ok) {
        return null;
    }
    const data = await res.json();
    return data.name;
}

function formatBytes(bytes) {
	if (bytes === 0) return "0 B";
	const k = 1024;
	const sizes = ["B", "KB", "MB", "GB"];
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

(async () => {
	const platformInfo = await getPlatformArchAndBinary();
	const { platformDir, archDir, binaryName, suffix } = platformInfo;

	const namedVersion = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

	// For the NPX package, we always use the configured branch or the specified version
	const branchName = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

	// Build ZIP download URL
	const zipFileName = `${binaryName}-${platformDir}-${archDir}.zip`;
	const downloadUrl = `${BASE_URL}/${branchName}/${zipFileName}`;

	let lastError = null;
	let binaryWorking = false;

	// Use branch name for caching
	const tinglyBinDir = join(cacheDir(), "tingly-box", branchName, "bin");

	// Create the binary directory
	try {
		if (!existsSync(tinglyBinDir)) {
			mkdirSync(tinglyBinDir, { recursive: true });
		}
	} catch (mkdirError) {
		console.error(`❌ Failed to create directory ${tinglyBinDir}:`, mkdirError.message);
		process.exit(1);
	}

	// The extracted binary path
	const binaryPath = join(tinglyBinDir, `${binaryName}-${platformDir}-${archDir}${suffix}`);

	// If binary doesn't exist, download and extract ZIP
	if (!existsSync(binaryPath)) {
		await downloadAndExtractZip(downloadUrl, tinglyBinDir, binaryName);

		// Make sure the binary is executable
		if (process.platform !== "win32") {
			chmodSync(binaryPath, 0o755);
		}

		console.log(`✅ Downloaded and extracted to ${binaryPath}`);
	}

    // Test if the binary can execute
    // Debug: Show binary location
    console.log(`🔍 Executing binary: ${binaryPath}`);

    try {
        execFileSync(binaryPath, remainingArgs, {
            stdio: "inherit",
            encoding: 'utf8'
        });

        // If we reach here, the binary executed successfully
        binaryWorking = true;

        // If execFileSync completes without throwing, the binary exited with code 0
        // No need to explicitly exit here, let the script continue
    } catch (execError) {
        lastError = execError;
        binaryWorking = false;

        // Extract detailed error information
        const errorCode = execError.code;
        const errorSignal = execError.signal;
        const errorMessage = execError.message;
        const errorStatus = execError.status;

        // Create comprehensive error output
        console.error(`\n❌ Tingly-Box execution failed`);
        console.error(`┌─ Error Details:`);
        console.error(`│  Message: ${errorMessage}`);

        if (errorCode) {
            console.error(`│  Code: ${errorCode}`);
            // Provide specific guidance for common error codes
            switch (errorCode) {
                case 'ENOENT':
                    console.error(`│  └─ Binary not found at: ${binaryPath}`);
                    console.error(`│     Try removing the cached binary: rm -rf "${join(cacheDir(), 'tingly-box')}"`);
                    break;
                case 'EACCES':
                    console.error(`│  └─ Permission denied. Check binary permissions.`);
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

        // Provide additional help for common scenarios
        if (process.platform === "linux") {
            console.error(`\n💡 Linux Troubleshooting:`);
            console.error(`   • Check if required libraries are installed:`);
            console.error(`     - For glibc issues: try on a different Linux distribution`);
            console.error(`     - For missing dependencies: install required system packages`);
            console.error(`   • Try running with strace: strace -o trace.log "${binaryPath}"`);
        }

        // Suggest retry
        console.error(`\n🔄 To retry, run: npx tingly-box ${remainingArgs.join(' ')}`);
        console.error(`   Or clear cache first: rm -rf "${join(cacheDir(), 'tingly-box')}"`);
    }

    if (!binaryWorking) {
        // Exit with the binary's exit code if available, otherwise default to 1
        const exitCode = lastError.status !== undefined ? lastError.status : 1;
        process.exit(exitCode);
    }
})();
