import type { ModelRequestSummary } from '@/components/AILogViewer';

export const sameRow = (a: ModelRequestSummary, b: ModelRequestSummary): boolean => {
    const keys = new Set([...Object.keys(a), ...Object.keys(b)]) as Set<keyof ModelRequestSummary>;
    for (const key of keys) if (a[key] !== b[key]) return false;
    return true;
};

// Reuses the previous object for every row whose data did not change, so memoized
// rows skip rendering on a refresh that only touched a few of them.
export const reconcile = (shown: ModelRequestSummary[], latest: ModelRequestSummary[]): ModelRequestSummary[] => {
    const previous = new Map(shown.map((row) => [row.request_id, row]));
    return latest.map((row) => {
        const old = previous.get(row.request_id);
        return old && sameRow(old, row) ? old : row;
    });
};

// While the reader is mid-interaction, requests that were not on screen are held
// back: the rows already shown keep their place and order (their data still
// updates in place), and the held ones are only counted.
export const holdNew = (shown: ModelRequestSummary[], latest: ModelRequestSummary[]): { shown: ModelRequestSummary[]; pending: number } => {
    const latestById = new Map(latest.map((row) => [row.request_id, row]));
    const shownIds = new Set(shown.map((row) => row.request_id));
    const updated = reconcile(shown, shown.map((row) => latestById.get(row.request_id) ?? row));
    return { shown: updated, pending: latest.reduce((n, row) => n + (shownIds.has(row.request_id) ? 0 : 1), 0) };
};
