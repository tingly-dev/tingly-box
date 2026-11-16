import { useCallback, useMemo, useRef, useSyncExternalStore } from 'react';
import { isFolder, type ProjectSummary } from '../client';
import { getProjectList, reloadProjectListSoon, subscribeProjectList } from './projectListStore';
import type { ProjectEvent } from './useProjectEvents';

/** The single list the sidebar groups and the waiting signal counts: every active project, of either type. */
export interface ProjectList {
    /** Every non-archived project and folder; null until the first answer. */
    all: ProjectSummary[] | null;
    /** `type=project`, starred first. */
    projects: ProjectSummary[] | null;
    /** `type=folder`, by name. */
    folders: ProjectSummary[] | null;
    /** How many threads wait on the user, in projects and folders alike. */
    waiting: number;
    /** Ask for a fresh list; coalesced with every other request in the debounce window. */
    reload: () => void;
}

/** Threads waiting on the user across a list of projects. */
export const waitingCount = (list: ProjectSummary[] | null): number => (list ?? []).reduce((n, p) => n + p.threads.waiting + p.threads.failed, 0);

/**
 * `GET /projects` (projects and folders together). Every instance reads the
 * one shared list (projectListStore), kept fresh by a single all-projects
 * event stream, a slow poll, a page reporting a change and the tab coming
 * back; none of them opens its own. `onEvent` and `onOpen` pass the stream's
 * events and reconnects on to a page that needs them too.
 */
export function useProjectList(enabled = true, onEvent?: (ev: ProjectEvent) => void, onOpen?: () => void): ProjectList {
    const forward = useRef(onEvent);
    forward.current = onEvent;
    const reopened = useRef(onOpen);
    reopened.current = onOpen;

    // Stable: a new function per render would leave and rejoin the store (and its stream) every time.
    const subscribe = useCallback(
        (notify: () => void) => (enabled ? subscribeProjectList(notify, (ev) => forward.current?.(ev), () => reopened.current?.()) : () => undefined),
        [enabled],
    );
    const all = useSyncExternalStore(subscribe, () => (enabled ? getProjectList() : null));

    const projects = useMemo(() => all && all.filter((p) => !isFolder(p)).sort((a, b) => Number(b.starred) - Number(a.starred)), [all]);
    const folders = useMemo(() => all && all.filter(isFolder).sort((a, b) => (a.name || a.path || '').localeCompare(b.name || b.path || '')), [all]);
    return { all, projects, folders, waiting: waitingCount(all), reload: reloadProjectListSoon };
}
