import { useCallback, useState } from 'react';

import { copyText } from '@/utils/clipboard';

// useCopyFeedback: copy text to the clipboard and flip a "copied" flag for a
// couple seconds so the caller can swap a tooltip/icon to acknowledge it.
// `onCopied` runs once the write actually succeeds — for callers that also
// fire a toast/notification alongside the visual flip.
export function useCopyFeedback(resetMs = 2000) {
    const [copied, setCopied] = useState(false);
    const copy = useCallback(
        (text: string, onCopied?: () => void) => {
            // copyText falls back to execCommand in non-secure contexts
            // (plain HTTP to a non-localhost host, e.g. over Tailscale).
            copyText(text)
                .then(() => {
                    setCopied(true);
                    onCopied?.();
                    setTimeout(() => setCopied(false), resetMs);
                })
                .catch(() => {
                    /* leave the flag unset; the copy simply didn't happen */
                });
        },
        [resetMs],
    );
    // For a caller that needs to clear the flag early — e.g. a dialog
    // resetting it on close instead of waiting out the timer.
    const reset = useCallback(() => setCopied(false), []);
    return { copied, copy, reset };
}
