/**
 * Copy text to the clipboard, working in both secure and non-secure contexts.
 *
 * `navigator.clipboard` is only available in a secure context — HTTPS, or
 * `http://localhost` / `http://127.0.0.1`. When the Web UI is served over plain
 * HTTP to a non-localhost host (a LAN IP, or a Tailscale `100.x` address) the
 * API is `undefined`, so every direct `navigator.clipboard.writeText(...)` call
 * throws. This helper falls back to the legacy `execCommand('copy')` via an
 * off-screen `<textarea>` in that case.
 *
 * Rejects if the copy could not be performed, so callers can surface an error.
 */
export async function copyText(text: string): Promise<void> {
    if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        return;
    }

    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-999999px';
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    try {
        const ok = document.execCommand('copy');
        if (!ok) {
            throw new Error('execCommand("copy") returned false');
        }
    } finally {
        document.body.removeChild(textArea);
    }
}
