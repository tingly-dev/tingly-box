import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProbeResult } from '@/types/probe';
import { runProbe } from './runProbe';

// useModelTestProbe: shared state for a single model card's test action. The
// trigger (bolt, hover-only) and the result indicator (persistent status dot)
// render in different corners of the card but need the same running/result
// state, so it's lifted into a hook instead of living inside one component.
export function useModelTestProbe(providerUuid: string, model: string) {
    const [running, setRunning] = useState(false);
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [dialogOpen, setDialogOpen] = useState(false);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    const run = useCallback(async () => {
        setRunning(true);
        setResult(null);
        const res = await runProbe({
            target_type: 'provider',
            provider_uuid: providerUuid,
            model,
            test_mode: 'streaming',
        });
        if (!mounted.current) return;
        setResult(res);
        setRunning(false);
    }, [providerUuid, model]);

    return {
        running,
        result,
        run,
        dialogOpen,
        openDialog: () => setDialogOpen(true),
        closeDialog: () => setDialogOpen(false),
    };
}
