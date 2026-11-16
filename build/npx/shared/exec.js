// Binary execution + failure diagnostics for npm launchers.

import { execFileSync } from "child_process";

// Run the resolved Go binary with inherited stdio. Returns on success; on
// failure prints a detailed diagnostic and exits with the binary's code.
// Optional hints: retryCmd (how the user re-invokes this shim) and cacheRoot
// (cache dir worth clearing on ENOENT).
export function execBinary(binaryPath, args, { retryCmd, cacheRoot } = {}) {
	try {
		execFileSync(binaryPath, args, {
			stdio: "inherit",
			encoding: 'utf8'
		});
		return;
	} catch (execError) {
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
					if (cacheRoot) {
						console.error(`│     Try removing the cached binary: rm -rf "${cacheRoot}"`);
					}
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

		if (retryCmd) {
			console.error(`\n🔄 To retry, run: ${retryCmd}`);
			if (cacheRoot) {
				console.error(`   Or clear cache first: rm -rf "${cacheRoot}"`);
			}
		}

		// Exit with the binary's exit code if available, otherwise default to 1
		const exitCode = errorStatus !== undefined && errorStatus !== null ? errorStatus : 1;
		process.exit(exitCode);
	}
}
