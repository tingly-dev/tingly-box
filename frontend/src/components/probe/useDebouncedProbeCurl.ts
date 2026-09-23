import {buildProbeCurl, type ProbeCurlResult} from './runProbe';
import type {ProbeRequest} from '@/types/probe';
import {useEffect, useMemo, useState} from 'react';

// useDebouncedProbeCurl: rebuild the curl preview 500 ms after the last change
// to `request` — pure construction, so debouncing just avoids redundant work,
// never a stale result. Shared by ProbeDialog's cURL section and BenchPage's
// live payload / preset-preview fetches, which differ only in which request
// they track.
export function useDebouncedProbeCurl(request: ProbeRequest | null): { data: ProbeCurlResult | null; loading: boolean } {
    const [data, setData] = useState<ProbeCurlResult | null>(null);
    const [loading, setLoading] = useState(false);
    const key = useMemo(() => JSON.stringify(request), [request]);
    useEffect(() => {
        if (!request) { setData(null); setLoading(false); return; }
        let cancelled = false;
        setLoading(true);
        const timer = setTimeout(async () => {
            const res = await buildProbeCurl(request);
            if (cancelled) return;
            setData(res);
            setLoading(false);
        }, 500);
        return () => { cancelled = true; clearTimeout(timer); };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);
    return { data, loading };
}
