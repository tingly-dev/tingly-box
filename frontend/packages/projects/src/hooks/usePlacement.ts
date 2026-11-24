import { useCallback, useState } from 'react';
import { readLocal, writeLocal } from '../utils/storage';

/** Where the Overview lives: in the side pane, in a floating window, or as a band under the header. */
export type OverviewPlacement = 'side' | 'window' | 'top';
/** A thread can replace the conversation, sit beside it, or float above it. */
export type ThreadPlacement = 'main' | 'side' | 'float';

const OVERVIEW: OverviewPlacement[] = ['side', 'window', 'top'];
const THREAD: ThreadPlacement[] = ['main', 'side', 'float'];

function read<T extends string>(key: string, allowed: T[], fallback: T): T {
    const v = readLocal(key);
    return allowed.includes(v as T) ? (v as T) : fallback;
}

/**
 * Where the Overview and an open thread are shown, remembered per browser
 * under `storageKey`. With the Overview on top, an explicitly side-placed thread sits below it. `overviewFloat` opens a thread
 * picked in the side-pane Overview as the floating window, leaving the Overview in place.
 */
export function usePlacement(storageKey = '') {
    const [overview, setOverviewState] = useState<OverviewPlacement>(() => read(`${storageKey}overviewPlacement`, OVERVIEW, 'side'));
    const [thread, setThreadState] = useState<ThreadPlacement>(() => read(`${storageKey}threadPlacement`, THREAD, 'main'));
    const setOverview = useCallback((v: OverviewPlacement) => {
        writeLocal(`${storageKey}overviewPlacement`, v);
        setOverviewState(v);
    }, [storageKey]);
    const setThread = useCallback((v: ThreadPlacement) => {
        writeLocal(`${storageKey}threadPlacement`, v);
        setThreadState(v);
    }, [storageKey]);
    // Off by default: the regular thread placement also applies from the Overview.
    const [overviewFloat, setOverviewFloatState] = useState(() => readLocal(`${storageKey}overviewFloat`) === '1');
    const setOverviewFloat = useCallback((v: boolean) => {
        writeLocal(`${storageKey}overviewFloat`, v ? '1' : '0');
        setOverviewFloatState(v);
    }, [storageKey]);
    return { overview, thread, overviewFloat, setOverview, setThread, setOverviewFloat };
}
