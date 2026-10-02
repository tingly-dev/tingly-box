#!/usr/bin/env node

import { execFileSync, spawn } from "child_process";
import { chmodSync, existsSync, mkdirSync } from "fs";
import { createRequire } from "module";
import { dirname, join } from "path";
import { fileURLToPath } from "url";
import { cacheDir } from "../shared/cachedir.js";
import { cleanupRetiredInstallDirs, cleanupStaleBinaryCaches } from "../shared/cleanup.js";
import { downloadAndExtractZip, extractZipFile } from "../shared/download.js";
import { findPlatformPackage, GUI_PLATFORM_PACKAGES } from "../shared/platform.js";
import { parseTransportVersion } from "../shared/transport.js";

// Configuration for binary downloads
const BASE_URL = "https://github.com/tingly-dev/tingly-box/releases/download";

// Default branch to use when not specified via transport version
// This will be replaced during the NPX build process
const BINARY_RELEASE_BRANCH = "latest";

const { version: VERSION, remainingArgs } = parseTransportVersion();

// This shim's own npm version; the platform package must carry the same one.
const OWN_VERSION = createRequire(import.meta.url)("./package.json").version;

// Same scheme as the cli shim: prefer the @tingly-dev/tingly-box-gui-<os>-<cpu>
// package npm installed next to this one (registry only, no GitHub), but only
// at exactly this version; otherwise download the baked release tag.
function resolveSource() {
	if (VERSION !== "latest") return { kind: "download", tag: VERSION };
	const local = findPlatformPackage(import.meta.url, GUI_PLATFORM_PACKAGES);
	if (local && local.version === OWN_VERSION) {
		return { kind: "package", tag: `v${local.version}`, local };
	}
	if (local) {
		console.warn(`⚠️  ${local.name}@${local.version} does not match tingly-box-gui@${OWN_VERSION}, downloading the release instead`);
	}
	return { kind: "download", tag: BINARY_RELEASE_BRANCH };
}

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
	} else if (platform === "win32" && process.arch !== "x64") {
		unsupported = {
			name: "Windows on " + process.arch,
			status: ["The desktop app is built for x64 Windows only"],
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

	// macOS (arm64) and Windows (x64) continue: install, then launch.
	const platformInfo = await getPlatformArchAndBinary();
	const { platformDir, archDir, binaryName, suffix, appName } = platformInfo;

	const source = resolveSource();
	// Cache dir is keyed by the release tag, whichever way the app arrives.
	const branchName = source.tag;

	const zipFileName = `${binaryName}-${platformDir}-${archDir}.zip`;
	const downloadUrl = `${BASE_URL}/${branchName}/${zipFileName}`;

	const cacheRoot = join(cacheDir(), "tingly-box-gui");
	const tinglyBinDir = join(cacheRoot, branchName, "bin");

	try {
		if (!existsSync(tinglyBinDir)) {
			mkdirSync(tinglyBinDir, { recursive: true });
		}
	} catch (mkdirError) {
		console.error(`❌ Failed to create directory ${tinglyBinDir}:`, mkdirError.message);
		process.exit(1);
	}

	// macOS: the .app bundle. Windows: the bare exe.
	const isMac = platform === "darwin";
	const appPath = isMac ? join(tinglyBinDir, appName) : join(tinglyBinDir, `tingly-box${suffix}`);

	if (!existsSync(appPath)) {
		if (source.kind === "package") {
			console.log(`📦 Installing the app from ${source.local.name}@${source.local.version}...`);
			await extractZipFile(source.local.zipPath, tinglyBinDir);
		} else {
			await downloadAndExtractZip(downloadUrl, tinglyBinDir);
		}
		if (!existsSync(appPath)) {
			console.error(`❌ The package did not contain ${appPath}`);
			process.exit(1);
		}

		if (isMac) {
			// Make sure the binary inside the .app bundle is executable
			const appBinaryPath = join(appPath, "Contents", "MacOS", "tingly-box");
			if (existsSync(appBinaryPath)) {
				chmodSync(appBinaryPath, 0o755);
			}
		}
		console.log(`✅ Installed to ${appPath}`);
	}

	// The app for this tag is in place — old tag dirs are now safe to GC.
	cleanupStaleBinaryCaches(cacheRoot, branchName);

	if (!isMac) {
		// Windows: start the exe detached so the shell prompt returns.
		console.log(`🚀 Launching ${appPath}...`);
		try {
			const child = spawn(appPath, [], { detached: true, stdio: "ignore" });
			child.on("error", (e) => {
				console.error(`\n❌ Failed to launch ${appPath}: ${e.message}`);
				console.error(`   Clear the cache and retry: rmdir /s /q "${cacheRoot}"`);
				process.exit(1);
			});
			child.unref();
		} catch (e) {
			console.error(`\n❌ Failed to launch ${appPath}: ${e.message}`);
			process.exit(1);
		}
		return;
	}

	console.log(`🔍 Launching app: ${appPath}`);

	// macOS: the app is only ad-hoc signed (no Developer ID). Extraction can
	// invalidate the bundle seal, so verify and re-sign ad hoc only when
	// needed. Neither npm nor this shim sets the quarantine flag, so
	// Gatekeeper never prompts on this path.
	try {
		execFileSync("codesign", ["--verify", "--deep", appPath], { stdio: "ignore" });
	} catch {
		try {
			console.log(`🔐 Signing app with ad-hoc signature...`);
			execFileSync("codesign", ["--force", "--deep", "--sign", "-", appPath], { stdio: "inherit" });
			console.log(`✅ App signed successfully`);
		} catch (signError) {
			console.error(`⚠️  Warning: Failed to sign app: ${signError.message}`);
			console.error(`    Continuing anyway...`);
		}
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
