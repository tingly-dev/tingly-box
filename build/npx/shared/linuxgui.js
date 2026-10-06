// Linux desktop-app preflight for the tingly-box-gui shim. The Linux zip holds
// a bare binary built with wails' `gtk3` tag, which links GTK 3 and
// WebKitGTK 4.1 dynamically (no static build is possible), so the shim checks
// for them before launching and, when they are missing, says which package to
// install instead of leaving a linker error. Rationale: .design/gui-packaging.md.

import { execFileSync } from "child_process";
import { readFileSync } from "fs";

// sonames the binary links (build/linux/Taskfile.yml build:gtk3).
export const REQUIRED_LIBS = ["libgtk-3.so.0", "libwebkit2gtk-4.1.so.0"];

// The libs of REQUIRED_LIBS absent from `ldconfig -p` output.
export function missingLibs(ldconfigOutput) {
	return REQUIRED_LIBS.filter((lib) => !ldconfigOutput.split("\n").some((l) => l.trim().startsWith(`${lib} `)));
}

// Missing libs on this host, or null when the cache cannot be read (no
// ldconfig, e.g. NixOS): the caller then just tries to launch.
export function checkLibs() {
	for (const bin of ["ldconfig", "/sbin/ldconfig", "/usr/sbin/ldconfig"]) {
		try {
			return missingLibs(execFileSync(bin, ["-p"], { encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }));
		} catch {
			// try the next location
		}
	}
	return null;
}

// The install command for the distribution, from /etc/os-release contents.
export function installHint(osRelease) {
	const field = (k) => (osRelease.match(new RegExp(`^${k}=("?)(.*)\\1$`, "m")) || [])[2] || "";
	const ids = `${field("ID")} ${field("ID_LIKE")}`.toLowerCase().split(/\s+/);
	const has = (...names) => names.some((n) => ids.includes(n));
	if (has("debian", "ubuntu")) return "sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0";
	if (has("fedora", "rhel", "centos")) return "sudo dnf install gtk3 webkit2gtk4.1";
	if (has("arch")) return "sudo pacman -S gtk3 webkit2gtk-4.1";
	if (has("suse", "opensuse")) return "sudo zypper install libgtk-3-0 libwebkit2gtk-4_1-0";
	return null;
}

export function readInstallHint() {
	try {
		return installHint(readFileSync("/etc/os-release", "utf8"));
	} catch {
		return null;
	}
}

// A desktop session is needed to open a window; over SSH or in a container
// neither variable is set.
export function hasDisplay(env = process.env) {
	return Boolean(env.DISPLAY || env.WAYLAND_DISPLAY);
}
