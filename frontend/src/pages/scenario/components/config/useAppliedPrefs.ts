import React from 'react';

export interface AppliedConfigResult {
    success: boolean;
    exists: boolean;
    preferences?: Record<string, any>;
    [key: string]: any;
}

interface UseAppliedPrefsOpts {
    open: boolean;
    /** Read back the config previously applied by the backend. */
    fetch: () => Promise<AppliedConfigResult | null | undefined>;
    /** Merge a saved config into state (saved prefs over defaults). */
    applySaved: (result: AppliedConfigResult) => void;
    /** Reset to defaults — used both when nothing was applied and on close. */
    applyDefaults: () => void;
    /** Extra close-time resets beyond `applyDefaults` (tool-specific state). */
    onClosed?: () => void;
    /** Extra reactive inputs that force a re-hydration while open (e.g. a
     * pending 1M-context change). Declared as a plain array by design. */
    deps?: readonly unknown[];
}

// Hydration-on-open shared by the config modals: on close, reset to defaults;
// on open, read back the config previously applied by the backend and merge
// the saved prefs over the defaults. First-time users (nothing applied yet)
// fall back to the defaults. While the readback is in flight the Quick tab
// shows a spinner instead of flashing the defaults — returned as
// isConfigLoading.
export const useAppliedPrefs = ({
    open,
    fetch,
    applySaved,
    applyDefaults,
    onClosed,
    deps = [],
}: UseAppliedPrefsOpts): boolean => {
    const [isConfigLoading, setIsConfigLoading] = React.useState(false);
    React.useEffect(() => {
        if (!open) {
            applyDefaults();
            onClosed?.();
            setIsConfigLoading(false);
            return;
        }
        let active = true;
        setIsConfigLoading(true);
        void fetch().then(result => {
            if (!active) return;
            if (result?.success && result.exists) {
                applySaved(result);
            } else {
                applyDefaults();
            }
        }).finally(() => {
            if (active) setIsConfigLoading(false);
        });
        return () => {
            active = false;
        };
        // `deps` is the caller's reactive input list by design.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, ...deps]);
    return isConfigLoading;
};
