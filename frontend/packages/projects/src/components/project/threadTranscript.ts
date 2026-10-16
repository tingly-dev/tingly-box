// A thread's events as transcript entries, so a thread renders with the
// shared transcript (@tingly/projects/transcript) instead of a view of its
// own. Threads record a little less than the engine's entries (no request id on a
// tool call, a request's answer kept on the request itself), so this fills
// the gaps: calls pair with their results in order, and a settled request
// gets the answer entry the transcript expects. A folder session also keeps
// the request id, the parent of a subagent's steps, task events and usage,
// which pass through as they are.
import type { ThreadEvent } from '../../client';
import type { TranscriptEntry } from '../../transcript';

interface RequestPayload {
    request_id: string;
    tool?: string;
    input?: Record<string, unknown>;
    state: 'pending' | 'allowed' | 'denied' | 'answered' | 'expired';
    answer?: string;
}

function parse<T>(s?: string): T | undefined {
    if (!s) return undefined;
    try {
        return JSON.parse(s) as T;
    } catch {
        return undefined;
    }
}

export interface ThreadTranscript {
    entries: TranscriptEntry[];
    /** The request the live turn waits on, if any. */
    pendingRequestId?: string;
}

/**
 * threadTranscript converts a thread's events. author names who sent a
 * message to the thread when it is worth saying (the coordinator); running
 * is whether a turn is live, so a pending request is still answerable.
 */
export function threadTranscript(events: ThreadEvent[], running: boolean, author: (from?: string) => string | undefined): ThreadTranscript {
    const entries: TranscriptEntry[] = [];
    // Calls without a result yet, oldest first: a result belongs to the
    // latest of them, as the engine writes one right after its call.
    const open: string[] = [];
    let pending: string | undefined;

    for (const e of events) {
        const timestamp = e.created_at;
        const first = entries.length;
        switch (e.kind) {
            case 'user':
                entries.push({ role: 'user', content: e.content, author: author(e.from), timestamp });
                break;
            case 'assistant':
                entries.push({ role: 'assistant', content: e.content, timestamp });
                break;
            case 'report': {
                // The turn's summary repeats its last reply: show it once.
                const last = entries.at(-1);
                if (last?.role === 'assistant' && !last.kind && last.content.trim() === e.content.trim()) break;
                entries.push({ role: 'assistant', content: e.content, timestamp });
                break;
            }
            case 'tool_use': {
                const id = e.request_id || `call:${e.id}`;
                open.push(id);
                entries.push({ kind: 'tool_use', content: e.content, request_id: id, payload: parse(e.payload), timestamp });
                break;
            }
            case 'tool_result':
                entries.push({ kind: 'tool_result', content: e.content, request_id: e.request_id || open.pop(), payload: parse(e.payload), timestamp });
                break;
            case 'task':
                entries.push({ kind: 'task', content: '', request_id: e.request_id, payload: parse(e.payload), timestamp });
                break;
            case 'usage':
                entries.push({ kind: 'usage', content: '', payload: parse(e.payload), timestamp });
                break;
            case 'approval':
            case 'ask': {
                const pl = parse<RequestPayload>(e.payload);
                if (!pl) break;
                const ask = e.kind === 'ask';
                entries.push(ask
                    ? { kind: 'ask_request', content: e.content, request_id: pl.request_id, timestamp }
                    : { kind: 'approval_request', content: pl.tool || e.content, request_id: pl.request_id, payload: pl.input, timestamp });
                if (pl.state === 'pending') {
                    if (running) pending = pl.request_id;
                } else if (pl.state !== 'expired') {
                    // An expired request has no answer: the transcript reads
                    // it as not answered.
                    entries.push({
                        kind: ask ? 'ask_response' : 'approval_response',
                        content: ask ? pl.answer ?? '' : pl.state === 'denied' ? 'denied' : 'approved',
                        request_id: pl.request_id,
                        timestamp,
                    });
                }
                break;
            }
            case 'error':
                entries.push({ kind: 'error', content: e.content, timestamp });
                break;
            default:
                entries.push({ kind: 'system', content: e.content, timestamp });
        }
        if (e.parent) for (let i = first; i < entries.length; i++) entries[i].parent = e.parent;
    }
    return { entries, pendingRequestId: pending };
}
