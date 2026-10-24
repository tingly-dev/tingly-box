// The transcript model: an agent session's flat transcript entries folded
// into what a page renders (tool calls paired with their results, approvals
// with their answers, a subagent's work under the call that ran it). It knows
// nothing about where the entries come from: each host passes in its own.

// TranscriptEntry is one entry of an agent session's transcript, as the
// engine writes it (agentboot/engine Entry). Kind "" is a plain chat message
// (role says whose); the other kinds are thinking, tool_use, tool_result,
// approval_request/_response, ask_request/_response, task, usage, system and
// error. request_id pairs a call with its result and a request with its
// answer; parent is the Agent call whose subagent wrote the entry.
export interface TranscriptEntry {
    role?: string;
    content: string;
    kind?: string;
    request_id?: string;
    payload?: unknown;
    parent?: string;
    // Who sent a user message, when the host knows more than "the user"
    // (a Projects thread is also written to by its coordinator).
    author?: string;
    // Files attached to a user message.
    attachments?: TranscriptAttachment[];
    // A user message added to a running turn that Claude has not read yet.
    pending?: boolean;
    // A question that answers itself with its suggestion at this time
    // unless the user answers first (ask_request).
    autoAt?: string;
    // The suggestion answered it, not the user (ask_response).
    auto?: boolean;
    timestamp: string;
}

export interface TranscriptAttachment {
    name: string;
    path: string;
    type: string;
    size: number;
}

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
    // The approval asked for right before this call ran: once settled, the
    // call's row says how it ended instead of a second line repeating it.
    request?: RequestBlock;
    // When the call was made.
    timestamp?: string;
}

// TaskEvent is the payload of a "task" transcript entry (agentboot/engine
// TaskEvent): one lifecycle event of a subagent or background
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
    timestamp?: string;
}

export type ActivityStep = ToolStep | ThinkingStep;

export type TranscriptBlock =
    | {type: 'user'; message: TranscriptEntry}
    | {type: 'assistant'; message: TranscriptEntry}
    | {type: 'activity'; steps: ActivityStep[]}
    // A subagent run: the Agent call, what the subagent did (its own
    // blocks), and its task state. The final report is its last reply.
    | {type: 'agent'; call: ToolStep; children: TranscriptBlock[]; task?: TaskState}
    // toolId is the call an approval was asked for (see ToolStep.request).
    | {type: 'request'; message: TranscriptEntry; response?: TranscriptEntry; toolId?: string}
    | {type: 'error'; message: TranscriptEntry}
    | {type: 'system'; message: TranscriptEntry};

export type RequestBlock = Extract<TranscriptBlock, {type: 'request'}>;

// Emitted once per turn by agentboot's converter
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
export const foldTasks = (messages: TranscriptEntry[]): Map<string, TaskState> => {
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
export const buildTranscript = (messages: TranscriptEntry[]): TranscriptBlock[] => {
    const tasks = foldTasks(messages);
    const byParent = new Map<string, TranscriptEntry[]>();
    const main: TranscriptEntry[] = [];
    const calls = messages.filter((m) => m.kind === 'tool_use' && m.request_id);
    const agentCalls = new Set(calls.filter((m) => isAgentTool(m.content)).map((m) => m.request_id));
    const loadedCalls = new Set(calls.map((m) => m.request_id));
    for (const m of messages) {
        if (m.parent && agentCalls.has(m.parent)) {
            const list = byParent.get(m.parent) ?? [];
            list.push(m);
            byParent.set(m.parent, list);
        } else if (m.parent && !loadedCalls.has(m.parent)) {
            // The call is in a part of the transcript that is not loaded (it
            // is read a page at a time): its steps show when that part does.
            continue;
        } else {
            // Output under a call that is not a subagent's stays visible
            // in the conversation rather than disappearing.
            main.push(m);
        }
    }
    return buildBlocks(main, byParent, tasks);
};

const buildBlocks = (messages: TranscriptEntry[], byParent: Map<string, TranscriptEntry[]>, tasks: Map<string, TaskState>): TranscriptBlock[] => {
    const blocks: TranscriptBlock[] = [];
    const requests = new Map<string, RequestBlock>();
    let activity: Extract<TranscriptBlock, {type: 'activity'}> | null = null;
    // Across blocks: an approval between a call and its result starts a new
    // activity row, and the result still belongs to the call before it.
    const tools = new Map<string, ToolStep>();
    // The entry before this one, among those that show: an approval right
    // after the call it gates belongs to that call.
    let previous: TranscriptEntry | undefined;

    const currentActivity = () => {
        if (!activity) {
            activity = {type: 'activity', steps: []};
            blocks.push(activity);
        }
        return activity;
    };

    for (const m of messages) {
        const before = previous;
        if (m.kind !== 'usage' && m.kind !== 'task') previous = m;
        switch (m.kind) {
            case 'thinking':
                currentActivity().steps.push({type: 'thinking', text: m.content, timestamp: m.timestamp});
                continue;
            case 'tool_use': {
                const id = m.request_id ?? '';
                const tool: ToolStep = {type: 'tool', id, name: m.content, input: m.payload, isError: false, task: tasks.get(id), timestamp: m.timestamp};
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
                    currentActivity().steps.push({type: 'tool', id: m.request_id ?? '', name: '', input: undefined, result: m.content, isError, timestamp: m.timestamp});
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
                const block: RequestBlock = {type: 'request', message: m};
                if (m.request_id) requests.set(m.request_id, block);
                // Claude Code asks right after announcing the call, so the
                // call just before, by the same tool and not yet run, is it.
                const tool = m.kind === 'approval_request' && before?.kind === 'tool_use' && before.request_id ? tools.get(before.request_id) : undefined;
                if (tool && tool.name === m.content && tool.result === undefined && !tool.request) {
                    tool.request = block;
                    block.toolId = tool.id;
                }
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

// agentStatus is the card's state. Without task events (older Claude Code)
// the call's result means it finished; a foreground run left "running"
// after its turn ended was cut off with the turn.
export const agentStatus = (block: Extract<TranscriptBlock, {type: 'agent'}>, turnLive: boolean): TaskState['status'] => {
    const status = block.task?.status ?? (block.call.result !== undefined ? 'completed' : 'running');
    if (status === 'running' && !turnLive && !block.task?.background) return 'stopped';
    return status;
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

// blockTime is when a block began: its message's time, or its first step's.
export const blockTime = (block: TranscriptBlock): string | undefined => {
    switch (block.type) {
        case 'activity':
            return block.steps.find((s) => s.timestamp)?.timestamp;
        case 'agent':
            return block.call.timestamp;
        default:
            return block.message.timestamp;
    }
};

// firstNewBlock is the index of the first block that began after seenAt,
// the place for a "New" divider; -1 when nothing is new or everything is
// (a divider above the whole transcript says nothing).
export const firstNewBlock = (blocks: TranscriptBlock[], seenAt?: string): number => {
    if (!seenAt) return -1;
    const seen = Date.parse(seenAt);
    if (Number.isNaN(seen)) return -1;
    const i = blocks.findIndex((b) => {
        const at = blockTime(b);
        return at !== undefined && Date.parse(at) > seen;
    });
    return i > 0 ? i : -1;
};

// WorkingVerb is what the live turn is doing, for the line under it.
export type WorkingVerb = 'thinking' | 'running' | 'reading' | 'editing' | 'searching' | 'delegating' | 'working';

const VERB_BY_TOOL: Record<string, WorkingVerb> = {
    Bash: 'running', BashOutput: 'running',
    Read: 'reading', Grep: 'reading', Glob: 'reading', LS: 'reading', NotebookRead: 'reading',
    Edit: 'editing', MultiEdit: 'editing', Write: 'editing', NotebookEdit: 'editing',
    WebFetch: 'searching', WebSearch: 'searching',
    Agent: 'delegating', Task: 'delegating',
};

// workingState reads the live turn from the transcript: what it is doing
// now (its last step that has not finished) and when it began (the last
// message that started it).
export const workingState = (blocks: TranscriptBlock[]): {verb: WorkingVerb; since?: string} => {
    let since: string | undefined;
    for (let i = blocks.length - 1; i >= 0; i--) {
        const b = blocks[i];
        if (b.type === 'user') {
            since = b.message.timestamp;
            break;
        }
    }
    const last = blocks.at(-1);
    let verb: WorkingVerb = 'working';
    if (last?.type === 'agent') verb = 'delegating';
    else if (last?.type === 'activity') {
        const step = last.steps.at(-1);
        if (step?.type === 'thinking') verb = 'thinking';
        else if (step?.type === 'tool') verb = step.result === undefined ? VERB_BY_TOOL[step.name] ?? 'working' : 'thinking';
    } else if (last?.type === 'user') verb = 'thinking';
    return {verb, since};
};

// Todo is one item of the agent's own progress list (Claude Code's TodoWrite).
export interface Todo {
    content: string;
    // The present-tense form shown while the item is in progress.
    activeForm?: string;
    status: 'pending' | 'in_progress' | 'completed';
}

// latestTodos is the progress list as of the last TodoWrite call: each call
// rewrites the whole list, so the last one is the current state. Undefined
// when the agent never wrote one.
export const latestTodos = (blocks: TranscriptBlock[]): Todo[] | undefined => {
    for (let i = blocks.length - 1; i >= 0; i--) {
        const b = blocks[i];
        if (b.type !== 'activity') continue;
        for (let j = b.steps.length - 1; j >= 0; j--) {
            const s = b.steps[j];
            if (s.type !== 'tool' || s.name !== 'TodoWrite') continue;
            const raw = (s.input as {todos?: unknown} | null)?.todos;
            if (!Array.isArray(raw)) continue;
            const todos = raw.flatMap((x): Todo[] => {
                const r = x as Partial<Todo> | null;
                if (!r || typeof r.content !== 'string' || r.content.trim() === '') return [];
                const status = r.status === 'completed' || r.status === 'in_progress' ? r.status : 'pending';
                return [{content: r.content, activeForm: typeof r.activeForm === 'string' ? r.activeForm : undefined, status}];
            });
            return todos.length > 0 ? todos : undefined;
        }
    }
    return undefined;
};

// RequestOutcome is how a request ended, or that it still waits.
export type RequestOutcome = 'pending' | 'allowed' | 'denied' | 'answered' | 'declined' | 'unanswered';

// requestOutcome reads the answer the engine wrote (an approval's is
// "approved" or "denied"; a question's is the answer, empty when declined).
// pending is whether this is the request the live turn waits on.
export const requestOutcome = (block: RequestBlock, pending: boolean): RequestOutcome => {
    if (pending) return 'pending';
    const {message, response} = block;
    if (!response) return 'unanswered';
    if (message.kind === 'ask_request') return response.content.trim() ? 'answered' : 'declined';
    return response.content === 'approved' ? 'allowed' : 'denied';
};

// AskQuestion is one question of an AskUserQuestion call: what is asked,
// its short tag, the options offered and whether several may be picked.
export interface AskOption {
    // label is what goes back to the agent; text is how it reads, without
    // the "(Recommended)" Claude Code appends to the option it suggests.
    label: string;
    text: string;
    description?: string;
    recommended: boolean;
}
export interface AskQuestion {
    question: string;
    header?: string;
    options: AskOption[];
    multiSelect: boolean;
}

const RECOMMENDED = /\s*\((recommended|推荐)\)\s*$/i;

// askQuestions reads the questions of an AskUserQuestion input; none when
// the input carries no structured questions (the card asks for words then).
export const askQuestions = (input: unknown): AskQuestion[] => {
    const questions = input && typeof input === 'object' ? (input as Record<string, unknown>).questions : undefined;
    if (!Array.isArray(questions)) return [];
    const out: AskQuestion[] = [];
    for (const q of questions) {
        if (!q || typeof q !== 'object') continue;
        const r = q as Record<string, unknown>;
        if (typeof r.question !== 'string' || !r.question.trim()) continue;
        const options: AskOption[] = [];
        for (const o of Array.isArray(r.options) ? r.options : []) {
            const label = o && typeof o === 'object' ? (o as Record<string, unknown>).label : undefined;
            if (typeof label !== 'string' || !label.trim()) continue;
            const description = (o as Record<string, unknown>).description;
            options.push({
                label,
                text: label.replace(RECOMMENDED, '') || label,
                description: typeof description === 'string' && description.trim() ? description : undefined,
                recommended: RECOMMENDED.test(label),
            });
        }
        out.push({
            question: r.question,
            header: typeof r.header === 'string' && r.header.trim() ? r.header : undefined,
            options,
            multiSelect: r.multiSelect === true,
        });
    }
    return out;
};

const EDIT_TOOLS = new Set(['Write', 'Edit', 'MultiEdit', 'NotebookEdit']);

export interface ApprovalSubject {
    // command: a shell command, shown whole; path / url: the file or page
    // the call touches; summary: the one field that best says what it does.
    kind: 'command' | 'path' | 'url' | 'summary';
    text: string;
}

// approvalSubject is the concrete thing an approval is about — what the user
// is actually saying yes to — instead of the tool's raw input.
export const approvalSubject = (tool: string, input: unknown): ApprovalSubject => {
    const record = input != null && typeof input === 'object' ? input as Record<string, unknown> : {};
    const field = (...keys: string[]) => {
        for (const key of keys) {
            const v = record[key];
            if (typeof v === 'string' && v.trim() !== '') return v.trim();
        }
        return '';
    };
    if (tool === 'Bash') {
        const command = field('command');
        if (command) return {kind: 'command', text: command};
    }
    if (EDIT_TOOLS.has(tool)) {
        const path = field('file_path', 'notebook_path', 'path');
        if (path) return {kind: 'path', text: path};
    }
    if (tool === 'WebFetch') {
        const url = field('url');
        if (url) return {kind: 'url', text: url};
    }
    return {kind: 'summary', text: toolSummary(input)};
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

export const formatTokens = (n: number): string => {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
    return String(n);
};

// RunSummary is what a run of calls did, counted by what a reader asks
// about: files read, searches, commands, files edited, anything else.
export interface RunSummary {
    read: number;
    searched: number;
    commands: number;
    edited: number;
    other: number;
    failed: number;
    // First to last step, when the steps carry times and it is at least a second.
    durationMs?: number;
}

// The todo list has its own card; its calls are left out of a run's rows and count.
export const CHECKLIST_TOOL = 'TodoWrite';

const READ_TOOLS = new Set(['Read']);
const SEARCH_TOOLS = new Set(['Grep', 'Glob']);
const RUN_COMMAND_TOOLS = new Set(['Bash', 'BashOutput']);
const RUN_EDIT_TOOLS = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit']);

export const summarizeRun = (steps: ActivityStep[]): RunSummary => {
    const sum: RunSummary = {read: 0, searched: 0, commands: 0, edited: 0, other: 0, failed: 0};
    const edited = new Set<string>();
    const times: number[] = [];
    for (const s of steps) {
        const at = s.timestamp ? Date.parse(s.timestamp) : NaN;
        if (!Number.isNaN(at)) times.push(at);
        if (s.type !== 'tool' || !s.name || s.name === CHECKLIST_TOOL) continue;
        // A denied call fails by definition; its row already says so.
        if (s.isError && !(s.request && requestOutcome(s.request, false) === 'denied')) sum.failed++;
        if (READ_TOOLS.has(s.name)) sum.read++;
        else if (SEARCH_TOOLS.has(s.name)) sum.searched++;
        else if (RUN_COMMAND_TOOLS.has(s.name)) sum.commands++;
        else if (RUN_EDIT_TOOLS.has(s.name)) {
            const input = (s.input ?? {}) as Record<string, unknown>;
            const path = input.file_path ?? input.notebook_path ?? input.path;
            edited.add(typeof path === 'string' && path ? path : s.id);
        } else sum.other++;
    }
    sum.edited = edited.size;
    if (times.length > 1) {
        const ms = Math.max(...times) - Math.min(...times);
        if (ms >= 1000) sum.durationMs = ms;
    }
    return sum;
};

// A run of calls folds into one line once it has this many.
export const FOLD_MIN_CALLS = 2;
