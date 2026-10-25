import { describe, expect, it } from 'vitest';
import type { ThreadEvent } from '../../client';
import { buildTranscript } from '../../transcript';
import { threadTranscript } from './threadTranscript';

let n = 0;
const ev = (kind: string, content: string, extra: Partial<ThreadEvent> = {}): ThreadEvent => ({
    id: `e${++n}`, thread_id: 't', kind, content, created_at: '2026-10-15T00:00:00Z', updated_at: '2026-10-15T00:00:00Z', ...extra,
});
const request = (state: string, extra: Record<string, unknown> = {}) => JSON.stringify({ request_id: `r${n}`, tool: 'Bash', input: { command: 'make test' }, state, ...extra });
const author = (e: ThreadEvent) => e.source === 'thread' ? 'Coordinator' : undefined;

describe('threadTranscript', () => {
    it('pairs calls with their results and folds a settled approval into its call', () => {
        const { entries, pendingRequestId } = threadTranscript([
            ev('user', 'Run the tests', { source: 'thread', source_ref: 'main' }),
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

describe('threadTranscript attachments', () => {
    it('carries the files of a user message', () => {
        const file = { name: 'a.png', path: 'uploads/x/a.png', type: 'image/png', size: 4 };
        const { entries } = threadTranscript([
            { id: '1', thread_id: 't', kind: 'user', content: '', payload: JSON.stringify({ attachments: [file] }), created_at: '2026-10-18T00:00:00Z', updated_at: '2026-10-18T00:00:00Z' },
        ], false, () => undefined);
        expect(entries[0].attachments).toEqual([file]);
    });

    it('marks a message added to a running turn until Claude has read it, and leaves held ones out', () => {
        const injected = ev('user', 'also add tests', { payload: '{"injected":true}' });
        const held = ev('queued', 'later', { payload: '{"state":"held"}' });
        const running = [ev('tool_use', 'Bash', { payload: '{}' }), injected, held];
        const pending = threadTranscript(running, true, author).entries;
        expect(pending.map((e) => e.content)).toEqual(['Bash', 'also add tests']);
        expect(pending[1].pending).toBe(true);

        const read = threadTranscript([...running, ev('tool_result', 'ok')], true, author).entries;
        expect(read[1].pending).toBe(false);
        // A subagent's step is not Claude's next step.
        const sub = threadTranscript([...running, ev('tool_result', 'ok', { parent: 'call-1' })], true, author).entries;
        expect(sub[1].pending).toBe(true);
        // Nothing runs any more: nothing is waiting to be read.
        expect(threadTranscript(running, false, author).entries[1].pending).toBe(false);
    });
});
