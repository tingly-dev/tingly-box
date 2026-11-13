// What the threads of a project wait on, readable from where the threads are
// listed: a card shows an approval or a question and answers it without
// opening the thread. The server is the authority on what is pending
// (GET /threads/{id}/requests); the project's event stream only says when to
// look again, and carries the new request or its answer so the card follows
// at once.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { ApiError, api, unwrap, type Thread, type ThreadEvent } from '../client';
import { threadOps } from '../components/project/threadOps';
import { threadTranscript } from '../components/project/threadTranscript';
import type { TranscriptEntry } from '../transcript';

/** One approval or question a thread waits on, as the transcript's own entry. */
export interface PendingRequest {
    id: string;
    kind: 'approval' | 'ask';
    entry: TranscriptEntry;
}

/** How a request ended when answered from a card: sent, or already settled elsewhere (another tab, the thread page, a chat). */
export type RespondResult = 'sent' | 'settled';

/** The answer a card gives; the same arguments the thread page's transcript passes. */
export type RespondRequest = (threadId: string, requestId: string, allow: boolean, answer: string, remember?: boolean, answers?: Record<string, string>) => Promise<RespondResult>;

/** The pending request a thread event carries; null for a settled one or another kind of entry. */
export function pendingRequestOf(event: ThreadEvent): PendingRequest | null {
    if (event.kind !== 'approval' && event.kind !== 'ask') return null;
    const entry = threadTranscript([event], true, () => undefined).entries[0];
    let state: string | undefined;
    try {
        state = (JSON.parse(event.payload ?? '') as { state?: string }).state;
    } catch {
        return null;
    }
    if (state !== 'pending' || !entry?.request_id) return null;
    return { id: entry.request_id, kind: event.kind, entry };
}

const EMPTY: PendingRequest[] = [];
const REVIEW_MS = 6000;

export interface ThreadRequests {
    /** What each thread waits on, oldest first; a thread that waits on nothing it can be answered for has no entry. */
    byThread: Record<string, PendingRequest[]>;
    /** Whether the thread's requests were read since it started waiting. */
    hydrated: (threadId: string) => boolean;
    respond: RespondRequest;
    /** A project stream frame: a request raised or settled. */
    observe: (event: ThreadEvent) => void;
    /** Reads again what every waiting thread asks (after the stream reconnects). */
    refresh: () => void;
    /** The thread whose card should open its requests (a notification's deep link). */
    reviewId: string | null;
    review: (threadId: string) => void;
    reviewed: () => void;
}

const Ctx = createContext<ThreadRequests | null>(null);

export const ThreadRequestsProvider = ({ value, children }: { value: ThreadRequests; children: ReactNode }) => <Ctx.Provider value={value}>{children}</Ctx.Provider>;

/** The requests of the project's threads; null where no page keeps them (a card then shows no requests). */
export const useThreadRequestsContext = (): ThreadRequests | null => useContext(Ctx);

/** What one thread waits on, empty outside a page that keeps requests. */
export function useThreadRequests(threadId: string): PendingRequest[] {
    return useContext(Ctx)?.byThread[threadId] ?? EMPTY;
}

/** Keeps the requests of `threads` that wait on the user. */
export function useThreadRequestsStore(threads: Thread[]): ThreadRequests {
    const [byThread, setByThread] = useState<Record<string, PendingRequest[]>>({});
    const [hydratedIds, setHydratedIds] = useState<Set<string>>(() => new Set());
    const [reviewId, setReviewId] = useState<string | null>(null);
    const [epoch, setEpoch] = useState(0);
    // Threads read since they started waiting, and the latest read per thread (an older answer must not overwrite a newer one).
    const requested = useRef(new Set<string>());
    const seq = useRef(new Map<string, number>());
    const reviewTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

    const read = useCallback(async (threadId: string) => {
        const mine = (seq.current.get(threadId) ?? 0) + 1;
        seq.current.set(threadId, mine);
        let list: PendingRequest[] | undefined;
        try {
            const res = await unwrap(api.GET('/api/v1/threads/{id}/requests', { params: { path: { id: threadId } } }));
            list = res.requests.map(pendingRequestOf).filter((r): r is PendingRequest => r != null);
        } catch {
            // Unreadable (the thread is gone, the server is down): the card keeps what it has and the thread page still answers.
        }
        if (seq.current.get(threadId) !== mine) return;
        if (list) setByThread((prev) => (list.length > 0 ? { ...prev, [threadId]: list } : without(prev, threadId)));
        setHydratedIds((prev) => (prev.has(threadId) ? prev : new Set(prev).add(threadId)));
    }, []);

    const waiting = useMemo(() => threads.filter((th) => th.status === 'waiting').map((th) => th.id).join(','), [threads]);
    useEffect(() => {
        const ids = new Set(waiting ? waiting.split(',') : []);
        for (const id of ids) {
            if (requested.current.has(id)) continue;
            requested.current.add(id);
            void read(id);
        }
        // A thread that no longer waits has nothing to answer.
        for (const id of [...requested.current]) {
            if (ids.has(id)) continue;
            requested.current.delete(id);
            seq.current.set(id, (seq.current.get(id) ?? 0) + 1);
            setByThread((prev) => without(prev, id));
            setHydratedIds((prev) => {
                if (!prev.has(id)) return prev;
                const next = new Set(prev);
                next.delete(id);
                return next;
            });
        }
    }, [waiting, epoch, read]);

    const observe = useCallback((event: ThreadEvent) => {
        if (event.kind !== 'approval' && event.kind !== 'ask') return;
        // A frame is newer than any read that began before it.
        seq.current.set(event.thread_id, (seq.current.get(event.thread_id) ?? 0) + 1);
        const pending = pendingRequestOf(event);
        let id = pending?.id;
        if (!id) {
            try {
                id = (JSON.parse(event.payload ?? '') as { request_id?: string }).request_id;
            } catch {
                return;
            }
        }
        if (!id) return;
        setByThread((prev) => {
            const list = prev[event.thread_id] ?? EMPTY;
            const rest = list.filter((r) => r.id !== id);
            if (!pending) return rest.length === list.length ? prev : rest.length > 0 ? { ...prev, [event.thread_id]: rest } : without(prev, event.thread_id);
            const next = [...rest, pending].sort((a, b) => a.entry.timestamp.localeCompare(b.entry.timestamp));
            return { ...prev, [event.thread_id]: next };
        });
    }, []);

    const refresh = useCallback(() => {
        requested.current.clear();
        setEpoch((n) => n + 1);
    }, []);

    const respond = useCallback<RespondRequest>(async (threadId, requestId, allow, answer, remember, answers) => {
        try {
            await threadOps(threadId).respond(requestId, allow, !!remember, answer, answers);
            return 'sent';
        } catch (e) {
            // 409: nothing waits on this request any more (answered in another tab, expired, the turn ended).
            if (!(e instanceof ApiError) || e.status !== 409) throw e;
            setByThread((prev) => {
                const rest = (prev[threadId] ?? EMPTY).filter((r) => r.id !== requestId);
                return rest.length > 0 ? { ...prev, [threadId]: rest } : without(prev, threadId);
            });
            void read(threadId);
            return 'settled';
        }
    }, [read]);

    const review = useCallback((threadId: string) => {
        setReviewId(threadId);
        clearTimeout(reviewTimer.current);
        reviewTimer.current = setTimeout(() => setReviewId(null), REVIEW_MS);
    }, []);
    const reviewed = useCallback(() => {
        clearTimeout(reviewTimer.current);
        setReviewId(null);
    }, []);
    useEffect(() => () => clearTimeout(reviewTimer.current), []);

    const hydrated = useCallback((threadId: string) => hydratedIds.has(threadId), [hydratedIds]);
    return useMemo(() => ({ byThread, hydrated, respond, observe, refresh, reviewId, review, reviewed }), [byThread, hydrated, respond, observe, refresh, reviewId, review, reviewed]);
}

function without(map: Record<string, PendingRequest[]>, key: string): Record<string, PendingRequest[]> {
    if (!(key in map)) return map;
    const { [key]: _gone, ...rest } = map;
    return rest;
}
