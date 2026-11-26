import { useCallback, useEffect, useRef, useState } from 'react';
import { writeToClipboard } from '../utils/clipboard';

export type CopyFeedbackState = 'idle' | 'copied' | 'failed';

// useCopyFeedback: copy text to the clipboard and flip a "copied" (or, when
// the write is refused — permissions, insecure context — a "failed") flag for
// a couple seconds so the caller can swap a tooltip/icon to acknowledge it.
// `onCopied` / `onFailed` run once the outcome is known, for callers that also
// fire a toast/notification alongside the visual flip. The timer is cleared
// on unmount so nothing updates a gone component.
export function useCopyFeedback(resetMs = 2000) {
    const [state, setState] = useState<CopyFeedbackState>('idle');
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    const copy = useCallback(
        async (text: string, onCopied?: () => void, onFailed?: () => void) => {
            try {
                await writeToClipboard(text);
            } catch {
                setState('failed');
                onFailed?.();
                window.clearTimeout(timer.current);
                timer.current = window.setTimeout(() => setState('idle'), resetMs);
                return;
            }
            setState('copied');
            onCopied?.();
            window.clearTimeout(timer.current);
            timer.current = window.setTimeout(() => setState('idle'), resetMs);
        },
        [resetMs],
    );
    // For a caller that needs to clear the flag early — e.g. a dialog
    // resetting it on close instead of waiting out the timer.
    const reset = useCallback(() => {
        window.clearTimeout(timer.current);
        setState('idle');
    }, []);
    return { copied: state === 'copied', failed: state === 'failed', copy, reset };
}
