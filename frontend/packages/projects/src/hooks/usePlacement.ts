import { useCallback, useState } from 'react';
import { readLocal, writeLocal } from '../utils/storage';

/** Where the Overview lives: in the side pane, in a floating window, or as a band under the header. */
export type OverviewPlacement = 'side' | 'window' | 'top';
/** Where an open thread lives: in the side pane or in a floating window. */
export type ThreadPlacement = 'side' | 'float';

const OVERVIEW: OverviewPlacement[] = ['side', 'window', 'top'];
const THREAD: ThreadPlacement[] = ['side', 'float'];

function read<T extends string>(key: string, allowed: T[], fallback: T): T {
    const v = readLocal(key);
    return allowed.includes(v as T) ? (v as T) : fallback;
}

/**
 * Where the Overview and an open thread are shown, remembered per browser
 * under `storageKey`. With the Overview on top the thread always sits below
 * it, so `thread` is only a preference then.
 */
export function usePlacement(storageKey = '') {
    const [overview, setOverviewState] = useState<OverviewPlacement>(() => read(`${storageKey}overviewPlacement`, OVERVIEW, 'side'));
    const [thread, setThreadState] = useState<ThreadPlacement>(() => read(`${storageKey}threadPlacement`, THREAD, 'side'));
    const setOverview = useCallback((v: OverviewPlacement) => {
        writeLocal(`${storageKey}overviewPlacement`, v);
        setOverviewState(v);
    }, [storageKey]);
    const setThread = useCallback((v: ThreadPlacement) => {
        writeLocal(`${storageKey}threadPlacement`, v);
        setThreadState(v);
    }, [storageKey]);
    return { overview, thread, setOverview, setThread };
}
