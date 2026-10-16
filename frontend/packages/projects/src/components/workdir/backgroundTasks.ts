// The background tasks of a folder session, read from its transcript: shell
// commands and subagents started with run_in_background, the work that
// outlives the turn that started it.
import type { BackgroundTask as LiveTask } from '../../client';
import { foldTasks, toolSummary, type TaskEvent, type TaskState, type TranscriptEntry } from '../../transcript';

/** One row of the background tasks panel. */
export interface BackgroundTaskRow extends TaskState {
    taskId: string;
    callId: string;
    /** The process that ran it is gone without saying how it ended (a server restart, an archive). */
    ended: boolean;
    /** What the call asked for: a command, or a subagent's prompt. */
    input?: { command?: string; prompt?: string; description?: string; subagent_type?: string };
    startedAt?: string;
    finishedAt?: string;
    /** A subagent's latest steps (newest last) and its latest reply. */
    recent?: { name: string; summary: string }[];
    reply?: string;
}

/** How many of a subagent's latest tool calls a row shows. */
const RECENT_STEPS = 5;

/**
 * backgroundTasks lists a session's background tasks, running ones first.
 * live is what the server reports running right now, which settles a task
 * whose process went away without a final event.
 */
export function backgroundTasks(entries: TranscriptEntry[], live: LiveTask[]): BackgroundTaskRow[] {
    const liveIds = new Set(live.map((t) => t.task_id));
    const calls = new Map<string, TranscriptEntry>();
    const finished = new Map<string, string>();
    const children = new Map<string, TranscriptEntry[]>();
    for (const m of entries) {
        if (m.kind === 'tool_use' && m.request_id) calls.set(m.request_id, m);
        if (m.kind === 'task' && m.request_id && ['task_notification', 'task_completed'].includes((m.payload as TaskEvent | undefined)?.event ?? '')) {
            finished.set(m.request_id, m.timestamp);
        }
        if (m.parent) {
            const list = children.get(m.parent) ?? [];
            list.push(m);
            children.set(m.parent, list);
        }
    }
    const rows: BackgroundTaskRow[] = [];
    const seen = new Set<string>();
    for (const [callId, t] of foldTasks(entries)) {
        if (!t.background || !t.taskId) continue;
        seen.add(t.taskId);
        const own = children.get(callId) ?? [];
        const replies = own.filter((m) => m.role === 'assistant' && !m.kind && m.content.trim());
        const call = calls.get(callId);
        rows.push({
            ...t, taskId: t.taskId, callId,
            ended: t.status === 'running' && !liveIds.has(t.taskId),
            input: call?.payload as BackgroundTaskRow['input'],
            startedAt: call?.timestamp,
            finishedAt: finished.get(callId),
            recent: own.filter((m) => m.kind === 'tool_use').slice(-RECENT_STEPS).map((m) => ({ name: m.content, summary: toolSummary(m.payload) })),
            reply: replies.at(-1)?.content,
        });
    }
    for (const t of live) {
        if (!seen.has(t.task_id)) {
            rows.push({ taskId: t.task_id, callId: '', status: 'running', background: true, taskType: t.task_type, description: t.description, ended: false });
        }
    }
    const rank = (r: BackgroundTaskRow) => (r.status === 'running' && !r.ended ? 0 : 1);
    return rows.map((r, i) => ({ r, i })).sort((a, b) => rank(a.r) - rank(b.r) || b.i - a.i).map(({ r }) => r);
}
