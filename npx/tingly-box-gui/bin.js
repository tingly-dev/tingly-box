#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, createWriteStream, existsSync, fsyncSync, mkdirSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { Readable } from "stream";
import { ProxyAgent } from "undici";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download/";

// GitHub API endpoint for getting latest release info
const LATEST_RELEASE_API_URL = "https://github.com/tingly-dev/tingly-box/releases/download/";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

// Create proxy agent from environment variables (HTTP_PROXY, HTTPS_PROXY)
// Only create ProxyAgent if proxy is configured, otherwise use undefined (direct connection)
const httpProxy = process.env.HTTP_PROXY || process.env.http_proxy;
const httpsProxy = process.env.HTTPS_PROXY || process.env.https_proxy;
const proxyUri = httpsProxy || httpProxy;
const dispatcher = proxyUri ? new ProxyAgent(proxyUri) : undefined;

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
	let suffix = "";
	let appName = "TinglyBox.app";  // macOS app bundle name

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

	return { platformDir, archDir, binaryName: "tingly-box-gui", suffix, appName };
}

async function downloadBinary(url, dest) {
	// console.log(`🔄 Downloading binary from ${url}...`);

	// Fetch with redirect following and optional proxy support
	const fetchOptions = {
		redirect: 'follow', // Automatically follow redirects
		headers: {
			'User-Agent': 'tingly-box-npx'
		}
	};
	// Only add dispatcher if proxy is configured
	if (dispatcher) {
		fetchOptions.dispatcher = dispatcher;
	}

	const res = await fetch(url, fetchOptions);

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

	// Fetch with redirect following and optional proxy support
	const fetchOptions = {
		redirect: 'follow',
		headers: {
			'User-Agent': 'tingly-box-npx'
		}
	};
	if (dispatcher) {
		fetchOptions.dispatcher = dispatcher;
	}

	const res = await fetch(url, fetchOptions);

	if (!res.ok) {
		console.error(`❌ Download failed: ${res.status} ${res.statusText}`);
		process.exit(1);
	}

	// Create a temporary file for the ZIP
	const zipPath = join(tmpdir(), `tingly-box-gui-${Date.now()}.zip`);
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
    const fetchOptions = {};
    if (dispatcher) {
        fetchOptions.dispatcher = dispatcher;
    }
    const res = await fetch(releaseUrl, fetchOptions);
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
	const platform = process.platform;

	// For Windows and Linux, show unsupported message
	if (platform === "win32" || platform === "linux") {
		const platformName = platform === "win32" ? "Windows" : "Linux";
		console.error(`\n❌ ${platformName} is not currently supported for tingly-box-gui`);
		console.error(`┌─ Status:`);
		console.error(`│  GUI version is currently only available for macOS`);
		console.error(`│  ${platformName} support is coming soon`);
		console.error(`└─ Platform: ${platform} (${process.arch})`);
		console.error(`\n💡 Alternatives:`);
		console.error(`   • Use the CLI version: npx tingly-box`);
		console.error(`   • Visit: https://github.com/tingly-dev/tingly-box for updates`);
		process.exit(1);
	}

	// For macOS, continue with app download and launch
	const platformInfo = await getPlatformArchAndBinary();
	const { platformDir, archDir, binaryName, appName } = platformInfo;

	const namedVersion = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

	// For the NPX package, we always use the configured branch or the specified version
	const branchName = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

	// Build ZIP download URL
	const zipFileName = `${binaryName}-${platformDir}-${archDir}.zip`;
	const downloadUrl = `${BASE_URL}/${branchName}/${zipFileName}`;

	// Use branch name for caching
	const tinglyBinDir = join(cacheDir(), "tingly-box-gui", branchName, "bin");

	// Create the binary directory
	try {
		if (!existsSync(tinglyBinDir)) {
			mkdirSync(tinglyBinDir, { recursive: true });
		}
	} catch (mkdirError) {
		console.error(`❌ Failed to create directory ${tinglyBinDir}:`, mkdirError.message);
		process.exit(1);
	}

	// The app bundle path
	const appPath = join(tinglyBinDir, appName);

	// If app doesn't exist, download and extract ZIP
	if (!existsSync(appPath)) {
		await downloadAndExtractZip(downloadUrl, tinglyBinDir, binaryName);
		console.log(`✅ Downloaded and extracted to ${appPath}`);
	}

	console.log(`🔍 Launching app: ${appPath}`);

	// Sign the app (macOS requires ad-hoc signing for downloaded apps)
	try {
		console.log(`🔐 Signing app with ad-hoc signature...`);
		execFileSync("codesign", ["--force", "--deep", "--sign", "-", appPath], {
			stdio: "inherit"
		});
		console.log(`✅ App signed successfully`);
	} catch (signError) {
		console.error(`⚠️  Warning: Failed to sign app: ${signError.message}`);
		console.error(`    Continuing anyway...`);
	}

	// Launch the app using `open` command
	try {
		console.log(`🚀 Launching ${appName}...`);
		// Detach the app by using open command
		execFileSync("open", ["-a", appPath], {
			stdio: "inherit"
		});
		console.log(`✅ ${appName} launched successfully!`);
	} catch (execError) {
		console.error(`\n❌ Failed to launch ${appName}`);
		console.error(`┌─ Error Details:`);
		console.error(`│  Message: ${execError.message}`);

		const errorCode = execError.code;
		if (errorCode) {
			console.error(`│  Code: ${errorCode}`);
		}

		const errorStatus = execError.status;
		if (errorStatus !== null && errorStatus !== undefined) {
			console.error(`│  Exit Code: ${errorStatus}`);
		}

		console.error(`└─ App Path: ${appPath}`);
		console.error(`   Platform: ${process.platform} (${process.arch})`);

		// Provide help
		console.error(`\n💡 Troubleshooting:`);
		console.error(`   • Try opening manually: open "${appPath}"`);
		console.error(`   • Check if the app is quarantined: xattr -l "${appPath}"`);
		console.error(`   • Remove quarantine if needed: xattr -cr "${appPath}"`);

		// Suggest retry
		console.error(`\n🔄 To retry, run: npx tingly-box-gui ${remainingArgs.join(' ')}`);
		console.error(`   Or clear cache first: rm -rf "${join(cacheDir(), 'tingly-box-gui')}"`);

		const exitCode = errorStatus !== undefined ? errorStatus : 1;
		process.exit(exitCode);
	}
})();
