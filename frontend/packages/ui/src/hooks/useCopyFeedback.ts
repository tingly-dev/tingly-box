import { useCallback, useEffect, useRef, useState } from 'react';
import { writeToClipboard } from '../utils/clipboard';

// useCopyFeedback: copy text to the clipboard and flip a "copied" flag for a
// couple seconds so the caller can swap a tooltip/icon to acknowledge it.
// `onCopied` runs once the write actually succeeds — for callers that also
// fire a toast/notification alongside the visual flip. A refused write
// (permissions, insecure context) leaves the flag alone, and the timer is
// cleared on unmount so nothing updates a gone component.
export function useCopyFeedback(resetMs = 2000) {
    const [copied, setCopied] = useState(false);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    const copy = useCallback(
        async (text: string, onCopied?: () => void) => {
            try {
                await writeToClipboard(text);
            } catch {
                return;
            }
            setCopied(true);
            onCopied?.();
            window.clearTimeout(timer.current);
            timer.current = window.setTimeout(() => setCopied(false), resetMs);
        },
        [resetMs],
    );
    // For a caller that needs to clear the flag early — e.g. a dialog
    // resetting it on close instead of waiting out the timer.
    const reset = useCallback(() => {
        window.clearTimeout(timer.current);
        setCopied(false);
    }, []);
    return { copied, copy, reset };
}
