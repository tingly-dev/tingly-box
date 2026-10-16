import { describe, expect, it } from 'vitest';
import type { ThreadEvent } from '../../client';
import { buildTranscript } from '../../transcript';
import { threadTranscript } from './threadTranscript';

let n = 0;
const ev = (kind: string, content: string, extra: Partial<ThreadEvent> = {}): ThreadEvent => ({
    id: `e${++n}`, thread_id: 't', kind, content, created_at: '2026-10-15T00:00:00Z', ...extra,
});
const request = (state: string, extra: Record<string, unknown> = {}) => JSON.stringify({ request_id: `r${n}`, tool: 'Bash', input: { command: 'make test' }, state, ...extra });
const author = (from?: string) => from === 'coordinator' ? 'Coordinator' : undefined;

describe('threadTranscript', () => {
    it('pairs calls with their results and folds a settled approval into its call', () => {
        const { entries, pendingRequestId } = threadTranscript([
            ev('user', 'Run the tests', { from: 'coordinator' }),
            ev('tool_use', 'Bash', { payload: '{"command":"make test"}' }),
            ev('approval', 'Run make test', { payload: request('allowed', { scope: 'thread' }) }),
            ev('tool_result', 'ok', { payload: '{"is_error":false}' }),
            ev('assistant', 'All green.'),
            ev('report', 'All green.'),
        ], false, author);
        expect(pendingRequestId).toBeUndefined();
        expect(entries[0]).toMatchObject({ role: 'user', author: 'Coordinator' });
        const blocks = buildTranscript(entries);
        expect(blocks.map((b) => b.type)).toEqual(['user', 'activity', 'request', 'assistant']);
        const activity = blocks[1];
        expect(activity.type === 'activity' && activity.steps[0]).toMatchObject({ name: 'Bash', result: 'ok', request: { response: { content: 'approved' } } });
    });

    it('leaves a request answerable only while its turn runs, and an expired one unanswered', () => {
        const pending = [ev('approval', 'Run make test', { payload: request('pending') })];
        expect(threadTranscript(pending, true, author).pendingRequestId).toBe(JSON.parse(pending[0].payload!).request_id);
        expect(threadTranscript(pending, false, author).pendingRequestId).toBeUndefined();
        const { entries } = threadTranscript([ev('approval', 'x', { payload: request('expired') })], false, author);
        expect(entries.map((e) => e.kind)).toEqual(['approval_request']);
    });

    it('keeps a question with its answer and a report that adds to the last reply', () => {
        const { entries } = threadTranscript([
            ev('ask', 'Which branch?', { payload: request('answered', { answer: 'main' }) }),
            ev('assistant', 'Done.'),
            ev('report', 'Merged into main.'),
            ev('system', 'Stopped.'),
        ], false, author);
        expect(entries.map((e) => [e.kind ?? e.role, e.content])).toEqual([
            ['ask_request', 'Which branch?'], ['ask_response', 'main'], ['assistant', 'Done.'], ['assistant', 'Merged into main.'], ['system', 'Stopped.'],
        ]);
    });

    it('keeps what a folder session records: call ids, a subagent\'s parent, task and usage events', () => {
        const { entries } = threadTranscript([
            ev('tool_use', 'Agent', { request_id: 'agent-1', payload: '{"description":"look"}' }),
            ev('task', '', { request_id: 'agent-1', payload: '{"event":"task_started","task_type":"local_agent"}' }),
            ev('tool_use', 'Read', { request_id: 'call-2', parent: 'agent-1', payload: '{"file_path":"a.go"}' }),
            ev('tool_result', 'package a', { request_id: 'call-2', parent: 'agent-1' }),
            ev('usage', '', { payload: '{"input_tokens":5}' }),
        ], false, author);
        expect(entries.map((e) => [e.kind, e.request_id, e.parent])).toEqual([
            ['tool_use', 'agent-1', undefined],
            ['task', 'agent-1', undefined],
            ['tool_use', 'call-2', 'agent-1'],
            ['tool_result', 'call-2', 'agent-1'],
            ['usage', undefined, undefined],
        ]);
        expect(entries[1].payload).toMatchObject({ event: 'task_started' });
    });
});
