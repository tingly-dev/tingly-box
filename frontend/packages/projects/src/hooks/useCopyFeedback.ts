import { useCallback, useEffect, useRef, useState } from 'react';
import { writeToClipboard } from '@tingly/ui';

/** Copies text and exposes a `copied` flag that resets after ~2s. */
export function useCopyFeedback() {
    const [copied, setCopied] = useState(false);
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    const copy = useCallback(async (text: string) => {
        try {
            await writeToClipboard(text);
        } catch {
            return;
        }
        setCopied(true);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => setCopied(false), 2000);
    }, []);
    return { copied, copy };
}
