import type {TaskEvent, TaskState} from '@tingly/projects/transcript';
import {foldTasks, toolSummary} from '@tingly/projects/transcript';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';

export type SessionStatus = SessionInfo['status'];

export const isBusyStatus = (status: string): boolean => status === 'running' || status === 'pending';

export const folderName = (path: string): string => path.split(path.startsWith('/') ? '/' : /[\\/]/).filter(Boolean).pop() || path;

// A session's title is the prompt that started it, the way a chat is named
// by its first message.
export const sessionTitle = (s: SessionInfo): string => s.request?.trim() || folderName(s.project);

export interface FolderGroup {
    path: string;
    sessions: SessionInfo[];
}

// groupSessionsByFolder keeps the incoming (most recent first) order both
// between folders and within each folder, so the folder worked in last is
// on top.
export const groupSessionsByFolder = (sessions: SessionInfo[]): FolderGroup[] => {
    const groups: FolderGroup[] = [];
    const byPath = new Map<string, FolderGroup>();
    for (const s of sessions) {
        let g = byPath.get(s.project);
        if (!g) {
            g = {path: s.project, sessions: []};
            byPath.set(s.project, g);
            groups.push(g);
        }
        g.sessions.push(s);
    }
    return groups;
};

export type {SessionUsage, TurnUsage} from '@/components/claudeCode/sessionUsage';
export {cacheHitPct, sessionUsage} from '@/components/claudeCode/sessionUsage';

export {timeAgo} from '@/utils/timeAgo';

// BackgroundTask is one row of the background tasks panel.
export interface BackgroundTask extends TaskState {
    taskId: string;
    callId: string;
    // The process that ran it is gone without saying how it ended (a server
    // restart, an archive): it isn't running any more, whatever was last said.
    ended: boolean;
    // What the call asked for: a command, or a subagent's prompt.
    input?: {command?: string; prompt?: string; description?: string; subagent_type?: string};
    startedAt?: string;
    finishedAt?: string;
    // A subagent's latest steps (newest last) and its latest reply.
    recent?: {name: string; summary: string}[];
    reply?: string;
}

// RECENT_STEPS is how many of a subagent's latest tool calls a row shows.
const RECENT_STEPS = 5;

// backgroundTasks lists the session's background tasks from its transcript,
// running ones first. live is what the backend reports running right now
// (SessionInfo.background_tasks), which settles a task whose process went
// away without a final event.
export const backgroundTasks = (
    messages: MessageInfo[],
    live: {task_id: string; task_type: string; description: string}[],
): BackgroundTask[] => {
    const liveIds = new Set(live.map((t) => t.task_id));
    const calls = new Map<string, MessageInfo>();
    const finished = new Map<string, string>();
    const children = new Map<string, MessageInfo[]>();
    for (const m of messages) {
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
    const rows: BackgroundTask[] = [];
    const seen = new Set<string>();
    for (const [callId, t] of foldTasks(messages)) {
        if (!t.background || !t.taskId) continue;
        seen.add(t.taskId);
        const ended = t.status === 'running' && !liveIds.has(t.taskId);
        const call = calls.get(callId);
        const own = children.get(callId) ?? [];
        const replies = own.filter((m) => m.role === 'assistant' && !m.kind && m.content.trim());
        rows.push({
            ...t, taskId: t.taskId, callId, ended,
            input: call?.payload as BackgroundTask['input'],
            startedAt: call?.timestamp,
            finishedAt: finished.get(callId),
            recent: own.filter((m) => m.kind === 'tool_use').slice(-RECENT_STEPS).map((m) => ({name: m.content, summary: toolSummary(m.payload)})),
            reply: replies[replies.length - 1]?.content,
        });
    }
    for (const t of live) {
        if (!seen.has(t.task_id)) {
            rows.push({taskId: t.task_id, callId: '', status: 'running', background: true, taskType: t.task_type, description: t.description, ended: false});
        }
    }
    const rank = (r: BackgroundTask) => (r.status === 'running' && !r.ended ? 0 : 1);
    return rows.map((r, i) => ({r, i})).sort((a, b) => rank(a.r) - rank(b.r) || b.i - a.i).map(({r}) => r);
};
