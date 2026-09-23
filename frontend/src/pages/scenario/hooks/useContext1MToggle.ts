import { useState } from 'react';

/**
 * Shared context-1M toggle plumbing for scenario pages: TemplatePage's
 * onContext1MToggle stores the pending change and opens the config panel so
 * the user can apply the matching env update; the panel clears it on close.
 *
 * Pages with an object-shaped payload (e.g. Claude Code scopes the change to
 * the toggled rule) keep their own state instead — this hook only covers the
 * plain boolean variant.
 */
export const useContext1MToggle = (openConfigPanel: () => void) => {
    const [pendingContext1MChange, setPendingContext1MChange] = useState<boolean | null>(null);

    const handleContext1MToggle = (newState: boolean) => {
        // Store the pending change and directly open config panel
        setPendingContext1MChange(newState);
        openConfigPanel();
    };

    const clearPendingContext1MChange = () => setPendingContext1MChange(null);

    return { pendingContext1MChange, handleContext1MToggle, clearPendingContext1MChange };
};
