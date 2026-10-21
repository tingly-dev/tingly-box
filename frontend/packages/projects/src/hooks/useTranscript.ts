import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, unwrap, type ThreadEvent } from '../client';
import type { SubagentSteps } from '../transcript';

/** How many of the latest events a thread opens with; older ones load on request. */
export const TAIL = 200;
/** Threads whose transcript is kept for going back to them. */
const CACHE_MAX = 20;

interface Transcript {
    /**
     * The loaded part of the conversation, every usage event (the status
     * line totals them), and the steps of the subagents that were opened.
     */
    events: ThreadEvent[];
    /** Older conversation events exist that are not loaded. */
    more: boolean;
    /** Subagents (by Agent call) whose steps were read, and whether older steps exist. */
    agents: Record<string, { more: boolean }>;
}


/** The transcripts seen on this page, by thread, most recent last. */
const cache = new Map<string, Transcript>();

function remember(threadId: string, t: Transcript) {
    cache.delete(threadId);
    cache.set(threadId, t);
    if (cache.size > CACHE_MAX) cache.delete(cache.keys().next().value!);
}

/** For tests. */
export const forgetTranscripts = () => cache.clear();

const time = (s: string) => new Date(s).getTime();

/**
 * The first loaded event of the conversation itself: what the window starts
 * at, and what the page before it is read from (usage events and subagent
 * steps are loaded apart from the window).
 */
const windowFirst = (list: ThreadEvent[]) => list.find((e) => !e.parent && e.kind !== 'usage');

/**
 * Adds or replaces events by id, in created order. An event older than the
 * loaded window (an old approval answered) is only replaced, never added: it
 * would sit at the top with a gap above it.
 */
export function mergeEvents(list: ThreadEvent[], incoming: ThreadEvent[]): ThreadEvent[] {
    if (incoming.length === 0) return list;
    const index = new Map(list.map((e, i) => [e.id, i]));
    const next = list.slice();
    const first = windowFirst(list);
    const start = first ? time(first.created_at) : -Infinity;
    let added = false;
    for (const e of incoming) {
        const i = index.get(e.id);
        if (i !== undefined) next[i] = e;
        else if (time(e.created_at) >= start) {
            index.set(e.id, next.length);
            next.push(e);
            added = true;
        }
    }
    // Stable: events written in the same instant keep their order.
    return added ? next.sort((a, b) => time(a.created_at) - time(b.created_at)) : next;
}

/** The latest write among the events, as a `since` cursor. */
function cursor(events: ThreadEvent[]): string | undefined {
    let max = -Infinity;
    for (const e of events) max = Math.max(max, time(e.updated_at ?? e.created_at));
    // Millisecond precision rounds down; since is inclusive, so nothing is missed.
    return Number.isFinite(max) ? new Date(max).toISOString() : undefined;
}

/** Events read in pages that may overlap what is loaded: added once each, in created order. */
const prepend = (list: ThreadEvent[], older: ThreadEvent[]) => {
    const have = new Set(list.map((e) => e.id));
    return [...older.filter((e) => !have.has(e.id)), ...list].sort((a, b) => time(a.created_at) - time(b.created_at));
};

const fetchEvents = (id: string, query: { parent?: string; limit?: number; before?: string; since?: string }) =>
    unwrap(api.GET('/api/v1/threads/{id}/events', { params: { path: { id }, query } }));

/**
 * One thread's transcript, read in pieces: the latest TAIL events on first
 * open, only what was added or changed since on a reconnect or a return to the
 * thread (kept per thread for the page's life), and older pages on request.
 * Live entries from the event stream go in with `apply`.
 */
export function useTranscript(threadId: string) {
    const [state, setState] = useState<{ threadId: string; t: Transcript | null }>(() => ({ threadId, t: cache.get(threadId) ?? null }));
    // Switching threads shows the cached transcript (or none) at once.
    if (state.threadId !== threadId) setState({ threadId, t: cache.get(threadId) ?? null });
    const current = useRef(threadId);
    current.current = threadId;
    // Live entries that came before the first read landed, so it cannot miss them.
    const early = useRef<ThreadEvent[]>([]);

    const update = useCallback((id: string, fn: (t: Transcript | null) => Transcript | null) => {
        if (id !== current.current) return;
        setState((s) => {
            if (s.threadId !== id) return s;
            const t = fn(s.t);
            if (t) remember(id, t);
            return { threadId: id, t };
        });
    }, []);

    /** Reads what was missed (everything recent when nothing is loaded yet). */
    const catchUp = useCallback(async () => {
        const id = current.current;
        if (!id) return;
        const have = cache.get(id);
        const since = have && cursor(have.events);
        if (since) {
            const r = await fetchEvents(id, { since });
            update(id, (t) => t && { ...t, events: mergeEvents(t.events, r.events) });
        } else {
            const r = await fetchEvents(id, { limit: TAIL });
            const pending = early.current.filter((e) => e.thread_id === id);
            early.current = [];
            update(id, (t) => ({ events: mergeEvents(mergeEvents(r.events, t?.events ?? []), pending), more: r.more, agents: t?.agents ?? {} }));
        }
    }, [update]);

    /** Loads the page before the first loaded event. */
    const loadEarlier = useCallback(async () => {
        const id = current.current;
        const first = windowFirst(cache.get(id)?.events ?? []);
        if (!first) return;
        const r = await fetchEvents(id, { limit: TAIL, before: first.id });
        update(id, (t) => t && { ...t, events: prepend(t.events, r.events), more: r.more });
    }, [update]);

    /** Reads a subagent's latest steps, or (earlier) the page before its first loaded one. */
    const loadAgent = useCallback(async (call: string, earlier = false) => {
        const id = current.current;
        const firstStep = cache.get(id)?.events.find((e) => e.parent === call);
        const r = await fetchEvents(id, { parent: call, limit: TAIL, ...(earlier && firstStep && { before: firstStep.id }) });
        update(id, (t) => t && { ...t, events: prepend(t.events, r.events), agents: { ...t.agents, [call]: { more: r.more } } });
    }, [update]);


    /** A live entry from the event stream. */
    const apply = useCallback((e: ThreadEvent) => {
        if (e.thread_id === current.current && !cache.has(e.thread_id)) early.current.push(e);
        update(e.thread_id, (t) => t && { ...t, events: mergeEvents(t.events, [e]) });
    }, [update]);

    /** Changes loaded events in place (streamed text). */
    const edit = useCallback((fn: (events: ThreadEvent[]) => ThreadEvent[]) => {
        const id = current.current;
        update(id, (t) => t && { ...t, events: fn(t.events) });
    }, [update]);

    useEffect(() => {
        if (threadId) catchUp().catch(() => undefined);
    }, [threadId, catchUp]);

    const t = state.threadId === threadId ? state.t : null;
    const agents = t?.agents;
    const subagents = useMemo<SubagentSteps>(() => ({
        steps: (call) => {
            const a = agents?.[call];
            return a ? { loaded: true, more: a.more } : { loaded: false };
        },
        load: loadAgent,
    }), [agents, loadAgent]);
    return { events: t?.events ?? null, more: t?.more ?? false, catchUp, loadEarlier, apply, edit, subagents };
}
