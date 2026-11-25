import { useEffect, useEffectEvent, useRef } from 'react';

/**
 * Calls `onVisible` when the page becomes visible after being hidden,
 * but only if at least `staleThresholdMs` have elapsed since the tab was last active.
 *
 * This provides a low-cost "re-sync on focus" for multi-tab scenarios without
 * requiring continuous polling.
 */
export function usePageVisibility(onVisible: () => void, staleThresholdMs = 30_000) {
  const hiddenAtRef = useRef<number | null>(null);
  const handleVisibility = useEffectEvent(() => {
    if (document.hidden) {
      hiddenAtRef.current = Date.now();
    } else {
      const hiddenAt = hiddenAtRef.current;
      if (hiddenAt !== null && Date.now() - hiddenAt >= staleThresholdMs) {
        onVisible();
      }
      hiddenAtRef.current = null;
    }
  });

  useEffect(() => {
    const listener = () => handleVisibility();
    document.addEventListener('visibilitychange', listener);
    return () => document.removeEventListener('visibilitychange', listener);
  }, []);
}
