// writeToClipboard copies text to the clipboard, falling back to a hidden
// textarea + execCommand when the async Clipboard API is unavailable
// (non-secure contexts, e.g. plain-HTTP LAN access). This is the single
// clipboard implementation in the repo — useCopyFeedback wraps it for
// component-level "copied" feedback.
export async function writeToClipboard(text: string): Promise<void> {
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
        document.execCommand('copy');
    } finally {
        document.body.removeChild(textArea);
    }
}
