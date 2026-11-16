// writeToClipboard copies text to the clipboard, falling back to a hidden
// textarea + execCommand when the async Clipboard API is unavailable
// (non-secure contexts, e.g. plain-HTTP LAN access). The fallback writes the
// text from the copy event: inside a MUI Dialog the focus trap takes focus
// back from the textarea, which then has no selection to copy, but the copy
// event still fires. A copy that wrote nothing rejects. This is the single
// clipboard implementation in the frontend — useCopyFeedback wraps it for
// component-level "copied" feedback.
export async function writeToClipboard(text: string): Promise<void> {
    if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        return;
    }
    let written = false;
    const onCopy = (event: ClipboardEvent) => {
        if (event.clipboardData) {
            event.clipboardData.setData('text/plain', text);
            event.preventDefault();
            written = true;
        }
    };
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-999999px';
    document.body.appendChild(textArea);
    document.addEventListener('copy', onCopy, true);
    textArea.focus();
    textArea.select();
    try {
        const ran = document.execCommand('copy');
        // Without copy events (a Firefox privacy setting) the browser copies
        // the textarea's selection, which only holds while it keeps focus.
        const copiedSelection = ran && document.activeElement === textArea;
        if (!written && !copiedSelection) {
            throw new Error('Copy command was rejected');
        }
    } finally {
        document.removeEventListener('copy', onCopy, true);
        document.body.removeChild(textArea);
    }
}
