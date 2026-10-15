import { useEffect, useState } from 'react';
import { anyProjectWaiting, PROJECTS_CHANGED_EVENT } from '@/services/projectsApi';

const POLL_MS = 30_000;

/**
 * Whether any project has a thread waiting on the user, for the dot on the
 * rail's Projects item. Reads the project list (which carries each project's
 * waiting count), again whenever the Projects pages change a project, on a
 * slow poll while the tab is visible, and when the tab comes back.
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
            anyProjectWaiting(ctrl.signal).then(setWaiting).catch(() => {
                // Unreachable or aborted: keep what the dot showed.
            });
        };
        check();
        const timer = window.setInterval(check, POLL_MS);
        window.addEventListener(PROJECTS_CHANGED_EVENT, check);
        document.addEventListener('visibilitychange', check);
        return () => {
            ctrl?.abort();
            window.clearInterval(timer);
            window.removeEventListener(PROJECTS_CHANGED_EVENT, check);
            document.removeEventListener('visibilitychange', check);
        };
    }, [enabled]);

    return enabled && waiting;
}
