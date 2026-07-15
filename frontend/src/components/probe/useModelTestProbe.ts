import { useState } from 'react';
import type { ProbeResult } from '@/types/probe';

// useModelTestProbe: shared state for a single model card's test action. The
// trigger (bolt, hover-only) and the result indicator (persistent status dot)
// render in different corners of the card but both need the same
// result/dialog-open state, so it's lifted into a hook instead of living
// inside one component. The actual probe request runs inside ProbeDialog
// (opened via `openDialog`); `setResult` is wired to ProbeDialog's
// `onResult` so a run — first or re-run — always lands back here.
export function useModelTestProbe() {
    const [result, setResult] = useState<ProbeResult | null>(null);
    const [dialogOpen, setDialogOpen] = useState(false);

    return {
        result,
        setResult,
        dialogOpen,
        openDialog: () => setDialogOpen(true),
        closeDialog: () => setDialogOpen(false),
    };
}
