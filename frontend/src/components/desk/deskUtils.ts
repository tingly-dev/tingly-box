import type {MessageInfo, SessionInfo} from '@/services/deskApi';

export type SessionStatus = SessionInfo['status'];

export const isBusyStatus = (status: string): boolean => status === 'running' || status === 'pending';

export const folderName = (path: string): string => path.split('/').filter(Boolean).pop() || path;

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

const SUMMARY_KEYS = ['command', 'file_path', 'path', 'pattern', 'url', 'query', 'description', 'prompt'];

// toolSummary picks the one input field that says what a tool call did (the
// command it ran, the file it touched), instead of dumping the whole input.
export const toolSummary = (input: unknown): string => {
    if (input == null || typeof input !== 'object') return '';
    const record = input as Record<string, unknown>;
    for (const key of SUMMARY_KEYS) {
        const v = record[key];
        if (typeof v === 'string' && v.trim() !== '') return v.trim().split('\n')[0];
    }
    const json = JSON.stringify(input);
    return json === '{}' ? '' : json;
};

export interface ToolStep {
    type: 'tool';
    id: string;
    name: string;
    input: unknown;
    result?: string;
    isError: boolean;
    // Set when the call started a background task (a backgrounded command).
    task?: TaskState;
}

// TaskEvent is the payload of a "task" transcript entry (internal/desk/
// convert.go taskEvent): one lifecycle event of a subagent or background
// command, keyed by the tool call that started it (the entry's request_id).
export interface TaskEvent {
    event: string;
    task_id?: string;
    task_type?: string;
    description?: string;
    subagent_type?: string;
    background?: boolean;
    status?: string;
    summary?: string;
    last_tool?: string;
    output_file?: string;
    usage?: {total_tokens: number; tool_uses: number; duration_ms: number};
    tasks?: {task_id: string; task_type?: string; description?: string}[];
    output?: string;
    truncated?: boolean;
}

// TaskState is a task's latest known state, folded from its events.
export interface TaskState {
    taskId?: string;
    taskType?: string; // local_bash, local_agent
    description?: string;
    status: 'running' | 'completed' | 'stopped' | 'failed';
    background: boolean;
    subagentType?: string;
    // What it is doing now, while running ("Running Search the codebase").
    activity?: string;
    lastTool?: string;
    summary?: string;
    outputFile?: string;
    // The end of a finished command's output, kept in the transcript
    // because the file it was written to is temporary.
    outputSnapshot?: {content: string; truncated: boolean};
    usage?: {total_tokens: number; tool_uses: number; duration_ms: number};
}

// The tools that run a subagent: Agent in current Claude Code, Task before.
export const isAgentTool = (name: string): boolean => name === 'Agent' || name === 'Task';

export interface ThinkingStep {
    type: 'thinking';
    text: string;
}

export type ActivityStep = ToolStep | ThinkingStep;

export type TranscriptBlock =
    | {type: 'user'; message: MessageInfo}
    | {type: 'assistant'; message: MessageInfo}
    | {type: 'activity'; steps: ActivityStep[]}
    // A subagent run: the Agent call, what the subagent did (its own
    // blocks), and its task state. The final report is its last reply.
    | {type: 'agent'; call: ToolStep; children: TranscriptBlock[]; task?: TaskState}
    | {type: 'request'; message: MessageInfo; response?: MessageInfo}
    | {type: 'error'; message: MessageInfo}
    | {type: 'system'; message: MessageInfo};

// Emitted once per turn by the backend's converter (internal/desk/convert.go)
// to record the Claude session id; it answers nothing the user asks, so the
// transcript leaves it out.
const SESSION_INIT_PREFIX = 'claude code session ';

const finalStatus = (status?: string): TaskState['status'] => {
    switch (status) {
        case 'completed':
            return 'completed';
        case 'stopped':
        case 'killed':
            return 'stopped';
        case 'failed':
        case 'error':
            return 'failed';
        default:
            return 'running';
    }
};

// foldTasks folds each tool call's task events into its latest state.
export const foldTasks = (messages: MessageInfo[]): Map<string, TaskState> => {
    const tasks = new Map<string, TaskState>();
    for (const m of messages) {
        if (m.kind !== 'task' || !m.request_id || m.payload == null) continue;
        const ev = m.payload as TaskEvent;
        const t = tasks.get(m.request_id) ?? {status: 'running', background: false};
        if (ev.task_id) t.taskId = ev.task_id;
        switch (ev.event) {
            case 'task_started':
                t.background = ev.background ?? false;
                t.subagentType = ev.subagent_type || t.subagentType;
                t.taskType = ev.task_type || t.taskType;
                t.description = ev.description || t.description;
                break;
            case 'output_file':
                t.outputFile = ev.output_file;
                break;
            case 'output_snapshot':
                t.outputSnapshot = {content: ev.output ?? '', truncated: ev.truncated ?? false};
                break;
            case 'task_progress':
                t.activity = ev.description;
                t.lastTool = ev.last_tool;
                if (ev.usage) t.usage = ev.usage;
                break;
            case 'task_updated':
                if (ev.status) t.status = finalStatus(ev.status);
                break;
            case 'task_notification':
            case 'task_completed':
                t.status = finalStatus(ev.status ?? 'completed');
                t.summary = ev.summary;
                t.outputFile = ev.output_file || t.outputFile;
                if (ev.usage) t.usage = ev.usage;
                t.activity = undefined;
                break;
        }
        tasks.set(m.request_id, t);
    }
    return tasks;
};

// buildTranscript turns the flat transcript into what the page renders:
// consecutive thinking and tool calls collapse into one activity block (each
// tool paired with its result by request_id), and an approval or question
// carries its answer instead of the answer showing as a separate line. A
// subagent's own entries (those with a parent) nest under the Agent call
// that ran it instead of mixing into the conversation.
export const buildTranscript = (messages: MessageInfo[]): TranscriptBlock[] => {
    const tasks = foldTasks(messages);
    const byParent = new Map<string, MessageInfo[]>();
    const main: MessageInfo[] = [];
    const agentCalls = new Set(messages.filter((m) => m.kind === 'tool_use' && isAgentTool(m.content) && m.request_id).map((m) => m.request_id));
    for (const m of messages) {
        // Output whose subagent call isn't in the transcript stays visible
        // in the conversation rather than disappearing.
        if (m.parent && agentCalls.has(m.parent)) {
            const list = byParent.get(m.parent) ?? [];
            list.push(m);
            byParent.set(m.parent, list);
        } else {
            main.push(m);
        }
    }
    return buildBlocks(main, byParent, tasks);
};

const buildBlocks = (messages: MessageInfo[], byParent: Map<string, MessageInfo[]>, tasks: Map<string, TaskState>): TranscriptBlock[] => {
    const blocks: TranscriptBlock[] = [];
    const requests = new Map<string, Extract<TranscriptBlock, {type: 'request'}>>();
    let activity: Extract<TranscriptBlock, {type: 'activity'}> | null = null;
    // Across blocks: an approval between a call and its result starts a new
    // activity row, and the result still belongs to the call before it.
    const tools = new Map<string, ToolStep>();

    const currentActivity = () => {
        if (!activity) {
            activity = {type: 'activity', steps: []};
            blocks.push(activity);
        }
        return activity;
    };

    for (const m of messages) {
        switch (m.kind) {
            case 'thinking':
                currentActivity().steps.push({type: 'thinking', text: m.content});
                continue;
            case 'tool_use': {
                const id = m.request_id ?? '';
                const tool: ToolStep = {type: 'tool', id, name: m.content, input: m.payload, isError: false, task: tasks.get(id)};
                if (id) tools.set(id, tool);
                if (isAgentTool(m.content)) {
                    // A subagent is a unit of work of its own, not one more
                    // step in a row: it gets a block, and splits the row.
                    activity = null;
                    blocks.push({type: 'agent', call: tool, children: buildBlocks(byParent.get(id) ?? [], byParent, tasks), task: tasks.get(id)});
                    continue;
                }
                currentActivity().steps.push(tool);
                continue;
            }
            case 'tool_result': {
                const tool = m.request_id ? tools.get(m.request_id) : undefined;
                const isError = Boolean((m.payload as {is_error?: boolean} | undefined)?.is_error);
                if (tool) {
                    tool.result = m.content;
                    tool.isError = isError;
                } else {
                    currentActivity().steps.push({type: 'tool', id: m.request_id ?? '', name: '', input: undefined, result: m.content, isError});
                }
                continue;
            }
            case 'usage':
                // Per-turn token accounting; the status line reads it, the
                // transcript doesn't show it (see sessionUsage).
                continue;
            case 'task':
                // Folded into the state of the call that started the task.
                continue;
            case 'approval_response':
            case 'ask_response': {
                const req = m.request_id ? requests.get(m.request_id) : undefined;
                if (req) req.response = m;
                continue;
            }
        }

        activity = null;
        switch (m.kind) {
            case 'approval_request':
            case 'ask_request': {
                const block: Extract<TranscriptBlock, {type: 'request'}> = {type: 'request', message: m};
                if (m.request_id) requests.set(m.request_id, block);
                blocks.push(block);
                break;
            }
            case 'error':
                blocks.push({type: 'error', message: m});
                break;
            case 'system':
                if (!m.content.startsWith(SESSION_INIT_PREFIX)) blocks.push({type: 'system', message: m});
                break;
            default:
                blocks.push({type: m.role === 'user' ? 'user' : 'assistant', message: m});
        }
    }
    return blocks;
};

// agentReport is what a subagent handed back: its last reply, else the
// summary Claude Code reported for it.
export const agentReport = (block: Extract<TranscriptBlock, {type: 'agent'}>): string | undefined => {
    for (let i = block.children.length - 1; i >= 0; i--) {
        const c = block.children[i];
        if (c.type === 'assistant' && c.message.content.trim()) return c.message.content;
    }
    return block.task?.summary;
};

// A request is answerable only while its turn is live and nothing has
// answered it yet: after an interrupt, a restart or an archive nothing is
// waiting for the answer any more.
export const pendingRequestId = (blocks: TranscriptBlock[], turnInFlight: boolean): string | undefined => {
    if (!turnInFlight) return undefined;
    for (let i = blocks.length - 1; i >= 0; i--) {
        const b = blocks[i];
        if (b.type === 'request' && !b.response && b.message.request_id) return b.message.request_id;
    }
    return undefined;
};

// TurnUsage is the payload of a "usage" transcript entry, written by the
// backend once per turn (internal/desk/convert.go turnUsage).
export interface TurnUsage {
    model?: string;
    input_tokens: number;
    output_tokens: number;
    cache_read_tokens: number;
    cache_write_tokens: number;
    context_tokens: number;
    context_window?: number;
    duration_ms?: number;
}

export interface SessionUsage {
    input: number;
    output: number;
    cacheRead: number;
    cacheWrite: number;
    // The latest turn: its model and how full its context was.
    latest: TurnUsage;
}

// sessionUsage totals a session's "usage" entries; undefined before any
// turn has reached the model.
export const sessionUsage = (messages: MessageInfo[]): SessionUsage | undefined => {
    let total: SessionUsage | undefined;
    for (const m of messages) {
        if (m.kind !== 'usage' || m.payload == null) continue;
        const u = m.payload as TurnUsage;
        total = {
            input: (total?.input ?? 0) + (u.input_tokens ?? 0),
            output: (total?.output ?? 0) + (u.output_tokens ?? 0),
            cacheRead: (total?.cacheRead ?? 0) + (u.cache_read_tokens ?? 0),
            cacheWrite: (total?.cacheWrite ?? 0) + (u.cache_write_tokens ?? 0),
            latest: u,
        };
    }
    return total;
};

// Share of prompt tokens served from the cache, the status line's "cache" figure.
export const cacheHitPct = (u: SessionUsage): number => {
    const prompt = u.input + u.cacheRead + u.cacheWrite;
    return prompt > 0 ? Math.round((u.cacheRead / prompt) * 100) : 0;
};

export const formatTokens = (n: number): string => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
    return String(n);
};

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
