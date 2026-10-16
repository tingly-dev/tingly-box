import { useCallback, useEffect, useState } from 'react';
import { api, unwrap, type Workdir } from '../../client';
import { PROJECTS_CHANGED } from '../../host/ProjectsHost';

const POLL_MS = 15_000;

/**
 * The working folders: the ones kept in the sidebar and every folder that has
 * a session, with what waits there. Refreshed when a page reports a change,
 * on a slow poll and when the tab comes back; null until the first answer.
 */
export function useWorkdirs(enabled = true): { workdirs: Workdir[] | null; reload: () => void } {
    const [workdirs, setWorkdirs] = useState<Workdir[] | null>(null);
    const [tick, setTick] = useState(0);
    useEffect(() => {
        if (!enabled) return;
        // Only the latest request may land, and none after unmount.
        let latest = 0;
        const load = () => {
            if (document.visibilityState === 'hidden') return;
            const request = ++latest;
            unwrap(api.GET('/api/v1/projects/workdirs'))
                .then((r) => {
                    if (request === latest) setWorkdirs(r.workdirs);
                })
                .catch(() => {
                    // Unreachable: keep the last list.
                });
        };
        load();
        const timer = window.setInterval(load, POLL_MS);
        window.addEventListener(PROJECTS_CHANGED, load);
        document.addEventListener('visibilitychange', load);
        return () => {
            latest = -1;
            window.clearInterval(timer);
            window.removeEventListener(PROJECTS_CHANGED, load);
            document.removeEventListener('visibilitychange', load);
        };
    }, [enabled, tick]);
    const reload = useCallback(() => setTick((x) => x + 1), []);
    return { workdirs, reload };
}
