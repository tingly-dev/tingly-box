// What waits on the user across every project and folder: the threads that
// wait on an answer or whose turn failed, and the approvals and questions
// they ask. The shared project list says which projects have any (its counts);
// those projects' threads are read once and then follow the all-projects
// stream's thread frames. That stream does not carry transcript entries, so a
// waiting thread's requests are read again whenever its frame arrives.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, unwrap, type ProjectSummary, type Thread } from '../client';
import { needsUser } from '../components/project/threadStatus';
import { useThreadRequestsStore, type ThreadRequests } from './threadRequests';
import { useProjectList } from './useProjectList';
import type { ProjectEvent } from './useProjectEvents';

export interface NeedsYou {
    /** Threads that need the user, waiting first, then the most recently updated. */
    threads: Thread[];
    /** The projects they belong to, by id. */
    projects: Map<string, ProjectSummary>;
    requests: ThreadRequests;
    /** False until the first read of the projects that have something waiting. */
    ready: boolean;
}

const order = (a: Thread, b: Thread) =>
    Number(b.status === 'waiting') - Number(a.status === 'waiting') || b.updated_at.localeCompare(a.updated_at);

export function useNeedsYou(): NeedsYou {
    const [threads, setThreads] = useState<Thread[]>([]);
    const [ready, setReady] = useState(false);
    const [epoch, setEpoch] = useState(0);
    const requests = useThreadRequestsStore(threads);
    const { reread, refresh } = requests;
    const latest = useRef<Map<string, ProjectSummary>>(new Map());

    const onEvent = useCallback((ev: ProjectEvent) => {
        if (ev.type !== 'thread') return;
        const th = ev.data as Thread;
        // A project's main thread is its conversation, not a row of work.
        if (th.id === latest.current.get(th.project_id)?.main_thread_id) return;
        const wanted = needsUser(th.status) && !th.archived_at;
        setThreads((list) => {
            const rest = list.filter((x) => x.id !== th.id);
            return wanted ? [...rest, th] : rest.length === list.length ? list : rest;
        });
        if (wanted && th.status === 'waiting') reread(th.id);
    }, [reread]);
    const onOpen = useCallback(() => {
        setEpoch((n) => n + 1);
        refresh();
    }, [refresh]);
    const { all } = useProjectList(true, onEvent, onOpen);

    const byId = useMemo(() => new Map((all ?? []).map((p) => [p.id, p])), [all]);
    latest.current = byId;

    // The projects with something that needs the user (the list's counts).
    const owners = useMemo(
        () => (all ?? []).filter((p) => p.threads.waiting + p.threads.failed > 0).map((p) => p.id).sort().join(','),
        [all],
    );
    useEffect(() => {
        if (all === null) return;
        const ids = owners ? owners.split(',') : [];
        if (ids.length === 0) {
            setThreads((list) => (list.length === 0 ? list : []));
            setReady(true);
            return;
        }
        let alive = true;
        void Promise.all(ids.map((id) => unwrap(api.GET('/api/v1/projects/{id}/threads', { params: { path: { id } } })).then((r) => r.threads, () => null)))
            .then((lists) => {
                if (!alive) return;
                const mine = new Set(ids);
                setThreads((prev) => {
                    // A project that could not be read keeps what it had.
                    const kept = prev.filter((th) => mine.has(th.project_id) && lists[ids.indexOf(th.project_id)] === null);
                    const read = lists.flatMap((l) => (l ?? []).filter((th) => needsUser(th.status) && !th.archived_at));
                    return [...kept, ...read];
                });
                setReady(true);
            });
        return () => { alive = false; };
        // `all` only gates the first read; the projects it names are `owners`.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [owners, epoch, all === null]);

    const sorted = useMemo(() => [...threads].sort(order), [threads]);
    return { threads: sorted, projects: byId, requests, ready };
}
