import { useEffect, useState } from 'react';
import { anythingWaiting, PROJECTS_CHANGED_EVENT } from '@/services/projectsApi';

/**
 * Whether a thread of a project or a working folder waits on the user,
 * for the dot on the rail's Projects item. Reads the project list
 * (folders included; it carries waiting counts), again whenever the Projects pages report
 * a change, and when the tab comes back (no background polling).
 */
export function useProjectsWaiting(enabled: boolean): boolean {
    const [waiting, setWaiting] = useState(false);

    useEffect(() => {
        if (!enabled) return;
        let ctrl: AbortController | undefined;
        const check = () => {
            if (document.visibilityState === 'hidden') return;
            ctrl?.abort();
            ctrl = new AbortController();
            anythingWaiting(ctrl.signal).then(setWaiting).catch(() => {
                // Unreachable or aborted: keep what the dot showed.
            });
        };
        check();
        window.addEventListener(PROJECTS_CHANGED_EVENT, check);
        document.addEventListener('visibilitychange', check);
        return () => {
            ctrl?.abort();
            window.removeEventListener(PROJECTS_CHANGED_EVENT, check);
            document.removeEventListener('visibilitychange', check);
        };
    }, [enabled]);

    return enabled && waiting;
}
