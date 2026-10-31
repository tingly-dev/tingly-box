import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '@/services/api';
import { getLocalMidnight, toLocalISOString } from '@/utils/datetime';

/** What the status row says about one agent: when it last called, and today's numbers. */
export interface AgentObservation {
    /** Timestamp of the newest request, ISO; null when the agent never called. */
    lastRequestAt: string | null;
    requestsToday: number;
    errorsToday: number;
}

/**
 * Read-only glance at an agent's traffic, from the existing usage endpoints
 * (the newest usage record + today's usage stats, both filtered by scenario) —
 * the same two calls AgentActivityDialog and Quick Start already make. There
 * is no combined "agent summary" endpoint; if one is added, this hook is the
 * only place to swap. `data` stays null until the first answer (so the row
 * does not flash "no requests" while loading) and after a failed load with
 * nothing earlier to show.
 */
export function useAgentObservation(scenario: string): { data: AgentObservation | null; refresh: () => Promise<void> } {
    const [data, setData] = useState<AgentObservation | null>(null);
    const seq = useRef(0);

    const refresh = useCallback(async () => {
        const mine = ++seq.current;
        try {
            const now = new Date();
            const [records, stats] = await Promise.all([
                api.getUsageRecords({ scenario, limit: 1 }),
                api.getUsageStats({
                    scenario,
                    group_by: 'scenario',
                    start_time: toLocalISOString(getLocalMidnight(now)),
                    end_time: toLocalISOString(now),
                    limit: 1,
                }),
            ]);
            if (mine !== seq.current) return;
            const first = Array.isArray(records?.data) ? records.data[0] : undefined;
            const row = Array.isArray(stats?.data) ? stats.data[0] : undefined;
            setData({
                lastRequestAt: first?.timestamp ?? null,
                requestsToday: row?.request_count ?? 0,
                errorsToday: row?.error_count ?? 0,
            });
        } catch {
            // Keep what was last known; with nothing, the row simply shows no numbers.
        }
    }, [scenario]);

    useEffect(() => {
        setData(null);
        void refresh();
        // No background polling: the numbers load with the page and again when
        // the window regains focus.
        const onFocus = () => void refresh();
        window.addEventListener('focus', onFocus);
        return () => {
            seq.current++;
            window.removeEventListener('focus', onFocus);
        };
    }, [refresh]);

    return { data, refresh };
}
