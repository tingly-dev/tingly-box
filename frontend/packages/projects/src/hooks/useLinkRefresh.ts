import { useCallback, useEffect, useRef, useState } from 'react';
import type { RefreshLinks } from '../client';

/** What a refresh did, summed over the items it asked about. */
export interface RefreshOutcome {
    done: number;
    recent: number;
    failed: number;
    /** Why, when something failed: the server's reason for the item or the request. */
    error?: string;
}

/**
 * A refresh of issues and PRs from GitHub: one call at a time, with whether it
 * is running and what it did. The readings themselves arrive on the thread
 * event, so this holds no data, only the person's own feedback.
 */
export function useLinkRefresh(call: () => Promise<RefreshLinks>) {
    const [busy, setBusy] = useState(false);
    const [outcome, setOutcome] = useState<RefreshOutcome | null>(null);
    const live = useRef(true);
    const running = useRef(false);
    useEffect(() => () => { live.current = false; }, []);
    const run = useCallback(async () => {
        if (running.current) return;
        running.current = true;
        setBusy(true);
        let next: RefreshOutcome;
        try {
            const { results } = await call();
            const failed = results.filter((r) => r.status === 'failed');
            next = {
                done: results.filter((r) => r.status === 'refreshed').length,
                recent: results.filter((r) => r.status === 'recent').length,
                failed: failed.length,
                error: failed.find((r) => r.error)?.error,
            };
        } catch (e) {
            next = { done: 0, recent: 0, failed: 1, error: e instanceof Error ? e.message : String(e) };
        }
        running.current = false;
        if (!live.current) return;
        setBusy(false);
        setOutcome(next);
    }, [call]);
    return { busy, outcome, run };
}

/** The sentence for an outcome, in the person's language. */
export function describeOutcome(o: RefreshOutcome | null, t: (key: string, opts?: Record<string, unknown>) => string): string {
    if (!o) return '';
    if (o.failed > 0) return o.done > 0 ? t('links.refreshedSome', { done: o.done, failed: o.failed }) : t('links.refreshFailed');
    if (o.done > 0) return o.done > 1 ? t('links.refreshedCount', { count: o.done }) : t('links.refreshed');
    return t('links.refreshRecent');
}
