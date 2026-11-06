import { useState } from 'react';
import { readLocal, writeLocal } from '../utils/storage';

// Pinned threads are a per-browser convenience, like the board's view choice:
// the key is scoped to the project, and a bad or missing value means none.
const pinKey = (projectId: string) => `overview.pinned.${projectId}`;

function readPins(projectId: string): string[] {
    try {
        const v: unknown = JSON.parse(readLocal(pinKey(projectId)) ?? '[]');
        return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
    } catch {
        return [];
    }
}

/** The project's thread ids pinned to the top of the Overview board, remembered per browser; newest pin first. */
export function usePinnedThreads(projectId: string): [string[], (id: string) => void] {
    // Kept per project: the pane can switch projects without remounting.
    const [pins, setPins] = useState<Record<string, string[]>>(() => ({ [projectId]: readPins(projectId) }));
    const ids = pins[projectId] ?? readPins(projectId);
    const toggle = (id: string) => setPins((cur) => {
        const curIds = cur[projectId] ?? readPins(projectId);
        const next = curIds.includes(id) ? curIds.filter((x) => x !== id) : [id, ...curIds];
        writeLocal(pinKey(projectId), JSON.stringify(next));
        return { ...cur, [projectId]: next };
    });
    return [ids, toggle];
}
