import { useEffect, useRef, useState, type RefObject } from 'react';

/**
 * Shared plumbing for the Weixin QR-bind and Feishu/Lark one-click
 * registration flows: both start a session against a (possibly not-yet-
 * created) bot, poll a status endpoint every few seconds, and must cancel
 * the pending session if the user navigates away before it completes.
 * Owns the temp bot UUID and the stoppedRef guard; callers own their own
 * state machine (start/poll callbacks, QR-specific states).
 */
export function useQrPollingSession(botUUID: string | undefined, cancel: (uuid: string) => Promise<any>) {
    // Generate a temporary UUID for a deferred bot (format: temp-{timestamp}-{random})
    const [tempUUID] = useState(() =>
        botUUID || `temp-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`
    );
    const effectiveBotUUID = botUUID || tempUUID;
    const stoppedRef = useRef(false);

    // Cancel the pending session if the user navigates away before completing.
    useEffect(() => {
        return () => {
            if (!stoppedRef.current && effectiveBotUUID) {
                cancel(effectiveBotUUID).catch(() => {});
            }
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return { effectiveBotUUID, stoppedRef };
}

/**
 * Polls `pollFn` every `intervalMs` while `active` is true, until it
 * resolves true (stop) or the session is marked stopped.
 */
export function usePollingLoop(
    active: boolean,
    stoppedRef: RefObject<boolean>,
    pollFn: () => Promise<boolean>,
    intervalMs = 2000
) {
    useEffect(() => {
        if (stoppedRef.current || !active) return;

        const interval = setInterval(async () => {
            const shouldStop = await pollFn();
            if (shouldStop) clearInterval(interval);
        }, intervalMs);

        return () => clearInterval(interval);
    }, [active, pollFn, intervalMs, stoppedRef]);
}
