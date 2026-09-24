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
}

export interface ThinkingStep {
    type: 'thinking';
    text: string;
}

export type ActivityStep = ToolStep | ThinkingStep;

export type TranscriptBlock =
    | {type: 'user'; message: MessageInfo}
    | {type: 'assistant'; message: MessageInfo}
    | {type: 'activity'; steps: ActivityStep[]}
    | {type: 'request'; message: MessageInfo; response?: MessageInfo}
    | {type: 'error'; message: MessageInfo}
    | {type: 'system'; message: MessageInfo};

// Emitted once per turn by the backend's converter (internal/desk/convert.go)
// to record the Claude session id; it answers nothing the user asks, so the
// transcript leaves it out.
const SESSION_INIT_PREFIX = 'claude code session ';

// buildTranscript turns the flat transcript into what the page renders:
// consecutive thinking and tool calls collapse into one activity block (each
// tool paired with its result by request_id), and an approval or question
// carries its answer instead of the answer showing as a separate line.
export const buildTranscript = (messages: MessageInfo[]): TranscriptBlock[] => {
    const blocks: TranscriptBlock[] = [];
    const requests = new Map<string, Extract<TranscriptBlock, {type: 'request'}>>();
    let activity: Extract<TranscriptBlock, {type: 'activity'}> | null = null;

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
            case 'tool_use':
                currentActivity().steps.push({
                    type: 'tool', id: m.request_id ?? '', name: m.content, input: m.payload, isError: false,
                });
                continue;
            case 'tool_result': {
                const act = currentActivity();
                const tool = act.steps.find((s): s is ToolStep => s.type === 'tool' && s.id !== '' && s.id === m.request_id);
                const isError = Boolean((m.payload as {is_error?: boolean} | undefined)?.is_error);
                if (tool) {
                    tool.result = m.content;
                    tool.isError = isError;
                } else {
                    act.steps.push({type: 'tool', id: m.request_id ?? '', name: '', input: undefined, result: m.content, isError});
                }
                continue;
            }
            case 'usage':
                // Per-turn token accounting; the status line reads it, the
                // transcript doesn't show it (see sessionUsage).
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
