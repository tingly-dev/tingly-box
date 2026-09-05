// Release-zip download + extraction shared by the cli and gui shims
// (the bundle shim extracts from packaged zips and has its own path).

import { chmodSync, createWriteStream, existsSync, mkdirSync, statSync } from "fs";
import { join } from "path";
import { Readable } from "stream";
import { ProxyAgent } from "undici";
import unzipper from "unzipper";

// Create proxy agent from environment variables (HTTP_PROXY, HTTPS_PROXY)
// Only create ProxyAgent if proxy is configured, otherwise use undefined (direct connection)
const httpProxy = process.env.HTTP_PROXY || process.env.http_proxy;
const httpsProxy = process.env.HTTPS_PROXY || process.env.https_proxy;
const proxyUri = httpsProxy || httpProxy;
export const dispatcher = proxyUri ? new ProxyAgent(proxyUri) : undefined;

export function formatBytes(bytes) {
	if (bytes === 0) return "0 B";
	const k = 1024;
	const sizes = ["B", "KB", "MB", "GB"];
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

// Print why the release download failed and what to do next, then exit.
// `hints` are package-specific next-step lines, printed verbatim (e.g.
// bundleSwitchHints()); the generic retry/proxy advice is always appended.
function failDownload(url, reason, hints) {
	console.error(`\n❌ Download failed: ${reason}`);
	console.error(`   URL: ${url}`);
	console.error(`\n💡 Next steps:`);
	for (const line of hints) {
		console.error(`   ${line}`);
	}
	console.error(`   • Retry later, or route the download through a proxy (HTTPS_PROXY).`);
	process.exit(1);
}

export async function downloadAndExtractZip(url, extractDir, { hints = [] } = {}) {
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

	let res;
	try {
		res = await fetch(url, fetchOptions);
	} catch (fetchError) {
		// Network-level failure (DNS, timeout, proxy refusal): surface the
		// underlying cause instead of an unhandled-rejection stack trace.
		const cause = fetchError.cause?.message || fetchError.message;
		failDownload(url, cause, hints);
	}

	if (!res.ok) {
		failDownload(url, `${res.status} ${res.statusText}`, hints);
	}

	const contentLength = res.headers.get("content-length");
	const totalSize = contentLength ? parseInt(contentLength, 10) : null;
	let downloadedSize = 0;

	// Convert the fetch response body to a Node.js readable stream
	const nodeStream = Readable.fromWeb(res.body);

	// Collect the entire ZIP into a buffer
	const chunks = [];
	for await (const chunk of nodeStream) {
		chunks.push(chunk);
		downloadedSize += chunk.length;
		if (totalSize) {
			const progress = ((downloadedSize / totalSize) * 100).toFixed(1);
			process.stdout.write(`\r⏱️ Downloading: ${progress}% (${formatBytes(downloadedSize)}/${formatBytes(totalSize)})`);
		} else {
			process.stdout.write(`\r⏱️ Downloaded: ${formatBytes(downloadedSize)}`);
		}
	}
	const zipBuffer = Buffer.concat(chunks);

	// Extract ZIP from buffer using unzipper
	try {
		console.log(`\n📦 Extracting ZIP to ${extractDir}...`);

		const directory = await unzipper.Open.buffer(zipBuffer);

		// Extract all files to the target directory
		for (const file of directory.files) {
			// Skip __MACOSX metadata
			if (file.path.startsWith('__MACOSX/') || file.path.includes('.DS_Store')) {
				continue;
			}

			const filePath = join(extractDir, file.path);

			// Materialize directory entries so empty dirs survive (.app bundles).
			if (file.type === 'Directory') {
				if (!existsSync(filePath)) {
					mkdirSync(filePath, { recursive: true });
				}
				continue;
			}
			// Get parent directory of the file in the ZIP
			const pathParts = file.path.split('/');
			pathParts.pop(); // Remove the filename
			const fileDir = pathParts.length > 0 ? join(extractDir, ...pathParts) : extractDir;

			console.log(`📄 Extracting: ${file.path} -> ${filePath}`);

			// Ensure parent directory exists
			if (fileDir !== extractDir && !existsSync(fileDir)) {
				mkdirSync(fileDir, { recursive: true });
			}

			// Remove existing directory if it exists (this was created incorrectly before)
			if (existsSync(filePath) && statSync(filePath).isDirectory()) {
				console.log(`🧹 Removing incorrect directory: ${filePath}`);
				// Can't easily remove a directory in Node without fs.rm (Node 14.14+)
				// Skip and let user clean up manually
				console.log(`⚠️  Please manually remove: rm -rf "${filePath}"`);
				continue;
			}

			// Extract file
			const content = await file.buffer();
			const fileStream = createWriteStream(filePath);
			await new Promise((resolve, reject) => {
				fileStream.write(content, (err) => {
					if (err) reject(err);
					else {
						fileStream.end();
						resolve();
					}
				});
			});
			// Set file permissions after writing
			if (process.platform !== "win32") {
				// Use ZIP permissions if available, otherwise default to 0o755 (executable)
				const permissions = file.unixPermissions && file.unixPermissions > 0 ? file.unixPermissions : 0o755;
				chmodSync(filePath, permissions);
			}
		}

		console.log(`✅ Extracted ZIP to ${extractDir}`);
	} catch (error) {
		console.error(`\n❌ Failed to extract ZIP: ${error.message}`);
		console.error(`Stack: ${error.stack}`);
		process.exit(1);
	}
}
