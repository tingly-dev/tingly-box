import {useEffect, useLayoutEffect, useRef} from 'react';

// Wait for each refresh to settle before scheduling another. Waking the tab
// or reconnecting refreshes immediately without creating overlapping polls.
export function useDeskPoll(refresh: (signal: AbortSignal) => Promise<unknown>, delay: number, enabled = true, scope?: string | null) {
    const latest = useRef(refresh);
    useLayoutEffect(() => { latest.current = refresh; }, [refresh]);
    useEffect(() => {
        if (!enabled) return;
        let disposed = false;
        let running = false;
        let wakePending = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        let controller: AbortController | undefined;
        const run = async () => {
            if (disposed) return;
            if (running) {
                wakePending = true;
                return;
            }
            clearTimeout(timer);
            running = true;
            controller = new AbortController();
            const deadline = setTimeout(() => controller?.abort(new DOMException('Desk refresh timed out', 'TimeoutError')), 15_000);
            try {
                await latest.current(controller.signal);
            } finally {
                clearTimeout(deadline);
                running = false;
                if (!disposed) {
                    timer = setTimeout(() => void run(), wakePending ? 0 : delay);
                    wakePending = false;
                }
            }
        };
        const wake = () => {
            if (document.visibilityState !== 'hidden') void run();
        };
        void run();
        window.addEventListener('online', wake);
        window.addEventListener('focus', wake);
        document.addEventListener('visibilitychange', wake);
        return () => {
            disposed = true;
            clearTimeout(timer);
            controller?.abort();
            window.removeEventListener('online', wake);
            window.removeEventListener('focus', wake);
            document.removeEventListener('visibilitychange', wake);
        };
    // A selection change must refresh immediately, even at the same cadence.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [delay, enabled, scope]);
}
