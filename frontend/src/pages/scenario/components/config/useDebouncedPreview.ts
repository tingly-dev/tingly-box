import React from 'react';

interface UseDebouncedPreviewOpts {
    open: boolean;
    /** Skip the preview entirely (e.g. Codex ChatGPT mode would render OAuth
     * tokens — the modal shows an info card instead). */
    enabled?: boolean;
    /** Debounce window; defaults to 250ms. */
    delay?: number;
    /** Reactive inputs that re-trigger the preview. Declared as a plain array
     * by design. */
    deps: readonly unknown[];
    /** The preview fetch itself; state writes happen in the caller's closure.
     * Rejections are swallowed so existing placeholders stay in place. */
    fetch: () => Promise<void>;
}

// Debounced server preview shared by the config modals: re-fetches `delay` ms
// after the inputs settle so dragging through Select options doesn't spam the
// backend. A cancelled flag drops late responses after the inputs (or the
// modal) change again.
export const useDebouncedPreview = ({
    open,
    enabled = true,
    delay = 250,
    deps,
    fetch,
}: UseDebouncedPreviewOpts): void => {
    React.useEffect(() => {
        if (!open || !enabled) return;
        let cancelled = false;
        const handle = setTimeout(async () => {
            try {
                if (!cancelled) await fetch();
            } catch {
                // Leave existing placeholders in place; the user can still
                // copy the base URL from the page itself.
            }
        }, delay);
        return () => {
            cancelled = true;
            clearTimeout(handle);
        };
        // `deps` is the caller's reactive input list by design.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, enabled, delay, ...deps]);
};
