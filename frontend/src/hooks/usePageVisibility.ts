import { useEffect, useRef } from 'react';

/**
 * Calls `onVisible` when the page becomes visible after being hidden,
 * but only if at least `staleThresholdMs` have elapsed since the tab was last active.
 *
 * This provides a low-cost "re-sync on focus" for multi-tab scenarios without
 * requiring continuous polling.
 *
 * @param onVisible - Callback to run when the tab regains focus and data is stale
 * @param staleThresholdMs - How long (ms) the tab must have been hidden before triggering (default 30s)
 */
export function usePageVisibility(onVisible: () => void, staleThresholdMs = 30_000) {
  const hiddenAtRef = useRef<number | null>(null);
  const onVisibleRef = useRef(onVisible);
  onVisibleRef.current = onVisible;

  useEffect(() => {
    const handleVisibility = () => {
      if (document.hidden) {
        hiddenAtRef.current = Date.now();
      } else {
        const hiddenAt = hiddenAtRef.current;
        if (hiddenAt !== null && Date.now() - hiddenAt >= staleThresholdMs) {
          onVisibleRef.current();
        }
        hiddenAtRef.current = null;
      }
    };

    document.addEventListener('visibilitychange', handleVisibility);
    return () => document.removeEventListener('visibilitychange', handleVisibility);
  }, [staleThresholdMs]);
}
