#!/usr/bin/env node

import { execFileSync } from "child_process";
import { chmodSync, existsSync, mkdirSync } from "fs";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { cacheDir } from "../shared/cachedir.js";
import { cleanupRetiredInstallDirs, cleanupStaleBinaryCaches } from "../shared/cleanup.js";
import { downloadAndExtractZip } from "../shared/download.js";
import { parseTransportVersion } from "../shared/transport.js";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

const { version: VERSION, remainingArgs } = parseTransportVersion();

async function getPlatformArchAndBinary() {
	const platform = process.platform;
	const arch = process.arch;

	let platformDir;
	let archDir;
	let suffix = "";
	let appName = "TinglyBox.app";  // macOS app bundle name

	if (platform === "darwin") {
		platformDir = "macos";
		archDir = "arm64"; // the only macOS GUI build; Intel exits earlier
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

(async () => {
	cleanupRetiredInstallDirs(dirname(fileURLToPath(import.meta.url)));

	const platform = process.platform;

	const releasesUrl = "https://github.com/tingly-dev/tingly-box/releases/latest";
	let unsupported = null;
	if (platform === "linux") {
		// Linux ships as distribution packages (they pull in GTK4/WebKitGTK),
		// not as an npx-launchable bundle.
		unsupported = {
			name: "Linux",
			status: [
				"Install the desktop app from the .deb / .rpm on the release page:",
				`  ${releasesUrl}`,
				"  sudo apt install ./tingly-box-gui-linux-amd64.deb   (Ubuntu 24.04+ / Debian 13+)",
				"  sudo dnf install ./tingly-box-gui-linux-amd64.rpm   (Fedora 40+)",
			],
		};
	} else if (platform === "win32") {
		unsupported = {
			name: "Windows",
			status: [`Download tingly-box-gui-windows-amd64.zip from ${releasesUrl}`],
		};
	} else if (platform === "darwin" && process.arch !== "arm64") {
		// Only an Apple Silicon build of the desktop app is published.
		unsupported = {
			name: "macOS on Intel",
			status: ["The desktop app is built for Apple Silicon Macs only"],
		};
	}
	if (unsupported) {
		console.error(`\n❌ ${unsupported.name} is not supported by npx tingly-box-gui`);
		console.error(`┌─ Status:`);
		for (const line of unsupported.status) console.error(`│  ${line}`);
		console.error(`└─ Platform: ${platform} (${process.arch})`);
		console.error(`\n💡 Alternatives:`);
		console.error(`   • Use the CLI version: npx tingly-box`);
		process.exit(1);
	}

	// For macOS, continue with app download and launch
	const platformInfo = await getPlatformArchAndBinary();
	const { platformDir, archDir, binaryName, appName } = platformInfo;

	// For the NPX package, we always use the configured branch or the specified version
	const branchName = VERSION === "latest" ? BINARY_RELEASE_BRANCH : VERSION;

	// Build ZIP download URL
	const zipFileName = `${binaryName}-${platformDir}-${archDir}.zip`;
	const downloadUrl = `${BASE_URL}/${branchName}/${zipFileName}`;

	// Use branch name for caching
	const cacheRoot = join(cacheDir(), "tingly-box-gui");
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

	// The app bundle path
	const appPath = join(tinglyBinDir, appName);

	// If app doesn't exist, download and extract ZIP
	if (!existsSync(appPath)) {
		await downloadAndExtractZip(downloadUrl, tinglyBinDir);

		// Make sure the binary inside the .app bundle is executable
		const appBinaryPath = join(appPath, "Contents", "MacOS", "tingly-box");
		if (existsSync(appBinaryPath)) {
			chmodSync(appBinaryPath, 0o755);
			console.log(`✅ Set executable permission on ${appBinaryPath}`);
		}

		console.log(`✅ Downloaded and extracted to ${appPath}`);
	}

	// The app for this tag is in place — old tag dirs are now safe to GC.
	cleanupStaleBinaryCaches(cacheRoot, branchName);

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
		console.error(`   Or clear cache first: rm -rf "${cacheRoot}"`);

		const exitCode = errorStatus !== undefined ? errorStatus : 1;
		process.exit(exitCode);
	}
})();
