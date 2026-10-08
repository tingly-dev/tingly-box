import type {TaskState, ToolStep, TranscriptBlock} from './deskUtils';
import {agentStatus, toolSummary} from './deskUtils';

// The trajectory is the transcript read for review: the same blocks the chat
// renders, one line per thing the agent did, so "what did it do" doesn't mean
// scrolling through every thought and every tool output. It adds no data —
// anything here is in the chat, and a row leads back to it.

export type TrajectoryKind = 'user' | 'tool' | 'agent' | 'approval' | 'ask' | 'error' | 'note' | 'reply' | 'system';

// Where a row's message sits in the chat: the tool call (activity row or
// subagent card), the approval/question card, or a top-level block.
export type TrajectoryAnchor = {call: string} | {request: string} | {block: number};

export interface TrajectoryRow {
    kind: TrajectoryKind;
    // 1-based; rows before the first user message are turn 0.
    turn: number;
    title: string;
    detail?: string;
    outcome?: string;
    failed?: boolean;
    // A subagent's own steps sit one level under its row.
    depth: number;
    anchor: TrajectoryAnchor;
    // Set on the user row that opens a turn.
    time?: string;
    // What a tool step changed, for the summary and its filter.
    file?: string;
    command?: boolean;
    status?: TaskState['status'];
}

export type TrajectoryFilter = 'files' | 'commands' | 'failures' | 'requests';

export interface TrajectorySummary {
    turns: number;
    files: string[];
    commands: number;
    failures: number;
    requests: number;
}

const EDIT_TOOLS = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit']);
const COMMAND_TOOLS = new Set(['Bash', 'BashOutput']);

const firstLine = (text: string): string => text.trim().split('\n').find((l) => l.trim() !== '')?.trim() ?? '';

// plainLine is a reply's first line without the Markdown that only makes
// sense rendered (headings, emphasis, inline code, link targets).
export const plainLine = (markdown: string): string => firstLine(markdown)
    .replace(/^#{1,6}\s+/, '')
    .replace(/^[-*+]\s+/, '')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/(\*\*|__|`)/g, '')
    .trim();

const editedFile = (step: ToolStep): string | undefined => {
    if (!EDIT_TOOLS.has(step.name)) return undefined;
    const input = (step.input ?? {}) as Record<string, unknown>;
    const path = input.file_path ?? input.notebook_path ?? input.path;
    return typeof path === 'string' && path.trim() ? path.trim() : undefined;
};

const toolRow = (step: ToolStep, turn: number, depth: number, anchor: TrajectoryAnchor): TrajectoryRow => ({
    kind: 'tool',
    turn,
    depth,
    anchor,
    title: step.name,
    detail: toolSummary(step.input) || undefined,
    failed: step.isError,
    file: editedFile(step),
    command: COMMAND_TOOLS.has(step.name),
});

// replyIndexes picks the reply of each turn: the last text before the next
// user message. Earlier text is the agent narrating its steps ("let me run
// the tests"), a note. While the turn still runs, text with work after it
// is not the reply yet.
const replyIndexes = (blocks: TranscriptBlock[], live: boolean): Set<number> => {
    const replies = new Set<number>();
    let last = -1;
    blocks.forEach((b, i) => {
        if (b.type === 'user') {
            if (last >= 0) replies.add(last);
            last = -1;
        } else if (b.type === 'assistant' && b.message.content.trim()) {
            last = i;
        }
    });
    if (last >= 0 && (!live || last === blocks.length - 1)) replies.add(last);
    return replies;
};

// rows flattens one run of blocks. A subagent's steps are anchored to its
// card: its inner rows only exist in the chat while the card is open.
const rows = (blocks: TranscriptBlock[], out: TrajectoryRow[], state: {turn: number}, depth: number, parent: TrajectoryAnchor | null, live: boolean) => {
    const replies = replyIndexes(blocks, live);
    blocks.forEach((b, i) => {
        const at = (own: TrajectoryAnchor): TrajectoryAnchor => parent ?? own;
        switch (b.type) {
            case 'user': {
                if (depth === 0) state.turn++;
                const title = firstLine(b.message.content);
                if (title) out.push({kind: 'user', turn: state.turn, depth, anchor: at({block: i}), title, time: depth === 0 ? b.message.timestamp : undefined});
                return;
            }
            case 'assistant': {
                const title = plainLine(b.message.content);
                if (title) out.push({kind: replies.has(i) ? 'reply' : 'note', turn: state.turn, depth, anchor: at({block: i}), title});
                return;
            }
            case 'activity':
                for (const s of b.steps) {
                    if (s.type === 'tool') out.push(toolRow(s, state.turn, depth, at({call: s.id})));
                }
                return;
            case 'agent': {
                const input = (b.call.input ?? {}) as {description?: string; subagent_type?: string};
                const anchor = at({call: b.call.id});
                const status = agentStatus(b, live);
                out.push({
                    kind: 'agent', turn: state.turn, depth, anchor,
                    title: input.subagent_type || b.task?.subagentType || b.call.name,
                    detail: input.description || b.task?.description,
                    status,
                    failed: status === 'failed',
                });
                rows(b.children, out, state, depth + 1, anchor, live);
                return;
            }
            case 'request': {
                const isAsk = b.message.kind === 'ask_request';
                out.push({
                    kind: isAsk ? 'ask' : 'approval', turn: state.turn, depth,
                    anchor: at(b.message.request_id ? {request: b.message.request_id} : {block: i}),
                    title: firstLine(b.message.content),
                    detail: isAsk ? undefined : toolSummary(b.message.payload) || undefined,
                    outcome: b.response ? firstLine(b.response.content) : undefined,
                });
                return;
            }
            case 'error':
                out.push({kind: 'error', turn: state.turn, depth, anchor: at({block: i}), title: firstLine(b.message.content), failed: true});
                return;
            case 'system': {
                const title = firstLine(b.message.content);
                if (title) out.push({kind: 'system', turn: state.turn, depth, anchor: at({block: i}), title});
                return;
            }
        }
    });
};

// trajectoryRows is the review projection of a transcript. `live` says a turn
// is running, so a subagent without a result yet reads as running, not cut off.
export const trajectoryRows = (blocks: TranscriptBlock[], live = false): TrajectoryRow[] => {
    const out: TrajectoryRow[] = [];
    rows(blocks, out, {turn: 0}, 0, null, live);
    return out;
};

export const matchesFilter = (row: TrajectoryRow, filter: TrajectoryFilter): boolean => {
    switch (filter) {
        case 'files':
            return row.file !== undefined;
        case 'commands':
            return row.command === true;
        case 'failures':
            return row.failed === true;
        case 'requests':
            return row.kind === 'approval' || row.kind === 'ask';
    }
};

export const trajectorySummary = (rows: TrajectoryRow[]): TrajectorySummary => {
    const files: string[] = [];
    for (const r of rows) if (r.file && !files.includes(r.file)) files.push(r.file);
    return {
        turns: rows.reduce((max, r) => Math.max(max, r.turn), 0),
        files,
        commands: rows.filter((r) => matchesFilter(r, 'commands')).length,
        failures: rows.filter((r) => matchesFilter(r, 'failures')).length,
        requests: rows.filter((r) => matchesFilter(r, 'requests')).length,
    };
};
