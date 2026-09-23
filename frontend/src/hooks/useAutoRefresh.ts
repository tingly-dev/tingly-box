import { useEffect } from 'react';

// useAutoRefresh: run `refresh` on a 5s interval while `enabled`. The effect
// re-subscribes when a `deps` entry changes so the interval closes over fresh
// inputs (e.g. filter state) — pass [] when `refresh` reads no reactive data.
export function useAutoRefresh(enabled: boolean, refresh: () => void, deps: readonly unknown[] = []) {
    useEffect(() => {
        if (!enabled) return;
        const id = setInterval(refresh, 5000);
        return () => clearInterval(id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [enabled, ...deps]);
}
