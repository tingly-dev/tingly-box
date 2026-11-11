// The project conversation's tool events as runs of the shared transcript's
// steps, so what the coordinator did between its replies renders with the
// same component a thread uses (ActivityRun). The events arrive in order; a
// run is the consecutive stretch of them between two shown entries, exactly
// how buildTranscript folds a thread's calls into activity blocks.
import type { ThreadEvent } from '../../client';
import { buildTranscript, type ActivityStep, type TranscriptEntry } from '../../transcript';

/** The events that are the coordinator's own work, not a shown message. */
const workKind = (kind: ThreadEvent['kind'] | undefined): boolean =>
    kind === 'tool_use' || kind === 'tool_result' || kind === 'task';

export interface ToolRun {
    /** The id of the event that opens the run; the pane renders there. */
    firstId: string;
    /** Every event the run covers, so the pane skips them. */
    ids: Set<string>;
    /** The calls paired with their results, as the transcript folds them. */
    steps: ActivityStep[];
}

function parse(s?: string): unknown {
    if (!s) return undefined;
    try {
        return JSON.parse(s);
    } catch {
        return undefined;
    }
}

/**
 * toolRuns folds the conversation's tool events, in order, into runs of
 * steps. covered lists every event a run swallowed (a run renders at its
 * first event; the rest would draw nothing on their own).
 */
export function toolRuns(events: ThreadEvent[]): { runs: ToolRun[]; covered: Set<string> } {
    const runs: ToolRun[] = [];
    const covered = new Set<string>();
    let current: ThreadEvent[] = [];
    const close = () => {
        if (current.length === 0) return;
        // The same fills threadTranscript does: a call pairs with its result
        // by request id, or by order when one is missing.
        const open: string[] = [];
        const entries: TranscriptEntry[] = current.map((e) => {
            const requestId = e.kind === 'tool_use'
                ? (e.request_id || `call:${e.id}`)
                : e.kind === 'tool_result'
                    ? (e.request_id || open.pop())
                    : e.request_id;
            if (e.kind === 'tool_use') open.push(requestId ?? '');
            return { kind: e.kind, content: e.content, request_id: requestId, payload: parse(e.payload), timestamp: e.created_at };
        });
        const steps = buildTranscript(entries).flatMap((b) => (b.type === 'activity' ? b.steps : []));
        if (steps.length > 0) {
            runs.push({ firstId: current[0].id, ids: new Set(current.map((e) => e.id)), steps });
            for (const e of current) covered.add(e.id);
        }
        current = [];
    };
    for (const e of events) {
        if (workKind(e.kind)) current.push(e);
        else close();
    }
    close();
    return { runs, covered };
}
