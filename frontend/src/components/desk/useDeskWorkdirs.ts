import {useEffect, useMemo, useState} from 'react';
import * as deskApi from '@/services/deskApi';
import type {SessionInfo} from '@/services/deskApi';
import {DESK_SESSIONS_EVENT} from './deskEvents';
import {groupSessionsByFolder} from './deskUtils';
import {useDeskProjects} from './useDeskProjects';

const POLL_MS = 10_000;

export interface DeskWorkdir {
    path: string;
    /** Sessions in this folder waiting on the user (an approval or a question). */
    waiting: number;
    sessions: SessionInfo[];
}

/**
 * Desk's working folders for navigation outside Desk's page: the saved
 * shortcuts first, then every folder with an open session, most recent
 * first — the same folders, in the same order, as Desk's own list. Sessions
 * come from Desk's page while it is open and a slow poll otherwise.
 */
export function useDeskWorkdirs(enabled: boolean): {workdirs: DeskWorkdir[]; sessions: SessionInfo[]} {
    const {projects} = useDeskProjects();
    const [sessions, setSessions] = useState<SessionInfo[]>([]);

    useEffect(() => {
        if (!enabled) return;
        let ctrl: AbortController | undefined;
        const load = () => {
            if (document.visibilityState === 'hidden') return;
            ctrl?.abort();
            ctrl = new AbortController();
            deskApi.listSessions(undefined, ctrl.signal).then(setSessions).catch(() => {
                // Unreachable or aborted: keep the last list.
            });
        };
        const onPublished = (e: Event) => setSessions((e as CustomEvent<SessionInfo[]>).detail);
        load();
        const timer = window.setInterval(load, POLL_MS);
        window.addEventListener(DESK_SESSIONS_EVENT, onPublished);
        document.addEventListener('visibilitychange', load);
        return () => {
            ctrl?.abort();
            window.clearInterval(timer);
            window.removeEventListener(DESK_SESSIONS_EVENT, onPublished);
            document.removeEventListener('visibilitychange', load);
        };
    }, [enabled]);

    const workdirs = useMemo(() => {
        const groups = groupSessionsByFolder(sessions.filter((s) => s.status !== 'closed'));
        const byPath = new Map(groups.map((g) => [g.path, g]));
        const ordered = [
            ...projects.map((path) => byPath.get(path) ?? {path, sessions: []}),
            ...groups.filter((g) => !projects.includes(g.path)),
        ];
        return ordered.map((g) => ({...g, waiting: g.sessions.filter((s) => s.awaiting_input).length}));
    }, [sessions, projects]);

    return {workdirs: enabled ? workdirs : [], sessions};
}
