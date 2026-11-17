import {useEffect, useState} from 'react';
import {api} from '@/services/api';
import type {LimitStatusEntry} from './LimitsEditor';

const POLL_MS = 4000;

/** Polls the live limits state (in-flight, window usage, pressure) while `enabled`. */
export function useLimitsStatus(enabled: boolean): LimitStatusEntry[] {
    const [entries, setEntries] = useState<LimitStatusEntry[]>([]);
    useEffect(() => {
        if (!enabled) return;
        let cancelled = false;
        const load = async () => {
            if (document.visibilityState === 'hidden') return; // nobody is looking
            try {
                const res = await api.getLimitsStatus();
                if (!cancelled && res?.success && Array.isArray(res.data)) setEntries(res.data);
            } catch {
                // status is a nicety; the editor works without it
            }
        };
        void load();
        const id = setInterval(load, POLL_MS);
        return () => {
            cancelled = true;
            clearInterval(id);
        };
    }, [enabled]);
    return entries;
}
