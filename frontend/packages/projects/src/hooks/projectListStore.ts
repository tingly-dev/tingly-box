import { api, unwrap, type Project, type ProjectSummary } from '../client';
import { PROJECTS_CHANGED } from '../host/ProjectsHost';
import { openEventStream, type ProjectEvent } from './useProjectEvents';

// The stream keeps the list fresh; the poll only catches what it may have dropped.
export const POLL_MS = 5 * 60_000;
export const RELOAD_DEBOUNCE_MS = 300;

/**
 * The one project list of the page: a module-level store, so the sidebar, the
 * folder page and the IM project directory (which may sit under different
 * host providers) read the same answer from a single event stream, poll and
 * request. Ref-counted: the first subscriber starts the stream and poll, the
 * last one stops them. Events update the rows they name in place; the list is
 * fetched again only for a project it does not have, on a reconnect, when the
 * tab comes back, on a page's report and on the slow poll.
 */
type RowUpdate = (p: ProjectSummary) => ProjectSummary | null;

interface Store {
    list: ProjectSummary[] | null;
    subscribers: number;
    changeListeners: Set<() => void>;
    eventListeners: Set<(ev: ProjectEvent) => void>;
    openListeners: Set<() => void>;
    latest: number;
    /**
     * While a fetch is on its way, the row updates applied meanwhile, by
     * project: its answer may predate them, so they are applied on top of it.
     */
    sinceFetch: Map<string, RowUpdate> | null;
    timer?: number;
    stop?: () => void;
}

const store: Store = {
    list: null,
    subscribers: 0,
    changeListeners: new Set(),
    eventListeners: new Set(),
    openListeners: new Set(),
    latest: 0,
    sinceFetch: null,
};

export const getProjectList = (): ProjectSummary[] | null => store.list;

/** Fetch now. Only the latest answer lands. */
export function reloadProjectList(): void {
    if (document.visibilityState === 'hidden') return;
    const request = ++store.latest;
    store.sinceFetch = new Map();
    unwrap(api.GET('/api/v1/projects'))
        .then((r) => {
            if (request !== store.latest) return;
            let list = r.projects;
            store.sinceFetch?.forEach((update, id) => {
                list = applyRow(list, id, update) ?? list;
            });
            store.sinceFetch = null;
            store.list = list;
            store.changeListeners.forEach((l) => l());
        })
        .catch(() => {
            // Unreachable: keep the last list.
            if (request === store.latest) store.sinceFetch = null;
        });
}

/**
 * Events, page reports and tab changes come in bursts (a turn writes many,
 * and a page reports each): one reload per debounce window, whoever asked.
 */
export function reloadProjectListSoon(): void {
    window.clearTimeout(store.timer);
    store.timer = window.setTimeout(reloadProjectList, RELOAD_DEBOUNCE_MS);
}

/** The list with one row updated (or removed); null when it does not have the row. */
function applyRow(list: ProjectSummary[], id: string, update: RowUpdate): ProjectSummary[] | null {
    const i = list.findIndex((p) => p.id === id);
    if (i < 0) return null;
    const row = update(list[i]);
    return row ? list.map((p, j) => (j === i ? row : p)) : list.filter((_, j) => j !== i);
}

/** Updates one row; false when the list does not have it. */
function patchRow(id: string, update: RowUpdate): boolean {
    const next = store.list && applyRow(store.list, id, update);
    if (!next) return false;
    store.list = next;
    const prev = store.sinceFetch?.get(id);
    store.sinceFetch?.set(id, prev ? (p) => { const r = prev(p); return r && update(r); } : update);
    store.changeListeners.forEach((l) => l());
    return true;
}

/** Applies a thread or project event to its row; anything it cannot apply is fetched. */
function applyEvent(ev: ProjectEvent) {
    if (ev.type === 'thread') {
        const counts = ev.threads;
        if (!counts || !patchRow(ev.project_id, (p) => ({ ...p, threads: counts }))) reloadProjectListSoon();
    } else if (ev.type === 'project') {
        const d = ev.data as Partial<Project>;
        // A memory change ({memory}) is not on the row.
        if (!d.id) return;
        if (!patchRow(d.id, (p) => (d.archived_at ? null : { ...p, ...d })) && !d.archived_at) reloadProjectListSoon();
    }
}

const onEvent = (ev: ProjectEvent) => {
    applyEvent(ev);
    store.eventListeners.forEach((l) => l(ev));
};

const onOpen = () => {
    reloadProjectList();
    store.openListeners.forEach((l) => l());
};

function start(): () => void {
    reloadProjectList();
    const poll = window.setInterval(reloadProjectList, POLL_MS);
    window.addEventListener(PROJECTS_CHANGED, reloadProjectListSoon);
    document.addEventListener('visibilitychange', reloadProjectListSoon);
    const close = openEventStream('/api/v1/projects/events', onEvent, onOpen);
    return () => {
        store.latest++;
        store.sinceFetch = null;
        window.clearInterval(poll);
        window.clearTimeout(store.timer);
        window.removeEventListener(PROJECTS_CHANGED, reloadProjectListSoon);
        document.removeEventListener('visibilitychange', reloadProjectListSoon);
        close();
    };
}

/** Joins the shared list; returns the leave function. */
export function subscribeProjectList(
    onChange: () => void,
    onProjectEvent?: (ev: ProjectEvent) => void,
    onReopen?: () => void,
): () => void {
    store.changeListeners.add(onChange);
    if (onProjectEvent) store.eventListeners.add(onProjectEvent);
    if (onReopen) store.openListeners.add(onReopen);
    if (store.subscribers++ === 0) store.stop = start();
    return () => {
        store.changeListeners.delete(onChange);
        if (onProjectEvent) store.eventListeners.delete(onProjectEvent);
        if (onReopen) store.openListeners.delete(onReopen);
        if (--store.subscribers === 0) {
            store.stop?.();
            store.stop = undefined;
        }
    };
}
