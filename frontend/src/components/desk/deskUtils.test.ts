import {describe, expect, it} from 'vitest';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {agentReport, backgroundTasks, buildTranscript, cacheHitPct, formatTokens, groupSessionsByFolder, pendingRequestId, sessionUsage, toolSummary} from './deskUtils';

const msg = (m: Partial<MessageInfo>): MessageInfo => ({content: '', timestamp: '2026-01-01T00:00:00Z', ...m} as MessageInfo);

describe('buildTranscript', () => {
    it('collapses consecutive thinking and tool calls into one activity block, pairing results by id', () => {
        const blocks = buildTranscript([
            msg({role: 'user', content: 'fix the bug'}),
            msg({kind: 'thinking', content: 'look at main.go'}),
            msg({kind: 'tool_use', content: 'Read', request_id: 't1', payload: {file_path: 'main.go'}}),
            msg({kind: 'tool_use', content: 'Bash', request_id: 't2', payload: {command: 'go test'}}),
            msg({kind: 'tool_result', content: 'package main', request_id: 't1', payload: {is_error: false}}),
            msg({kind: 'tool_result', content: 'FAIL', request_id: 't2', payload: {is_error: true}}),
            msg({role: 'assistant', content: 'Found it.'}),
        ]);

        expect(blocks.map((b) => b.type)).toEqual(['user', 'activity', 'assistant']);
        const activity = blocks[1];
        if (activity.type !== 'activity') throw new Error('expected activity');
        expect(activity.steps).toEqual([
            {type: 'thinking', text: 'look at main.go'},
            {type: 'tool', id: 't1', name: 'Read', input: {file_path: 'main.go'}, result: 'package main', isError: false},
            {type: 'tool', id: 't2', name: 'Bash', input: {command: 'go test'}, result: 'FAIL', isError: true},
        ]);
    });

    it('attaches an approval answer to its request instead of showing it separately', () => {
        const blocks = buildTranscript([
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1'}),
            msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
        ]);
        expect(blocks).toHaveLength(1);
        expect(blocks[0]).toMatchObject({type: 'request', response: {content: 'approved'}});
    });

    it('pairs a result with its call across the approval that came between them', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Bash', request_id: 't1', payload: {command: 'go test'}}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1'}),
            msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
            msg({kind: 'tool_result', content: 'ok', request_id: 't1', payload: {is_error: false}}),
        ]);
        expect(blocks.map((b) => b.type)).toEqual(['activity', 'request']);
        expect(blocks[0]).toMatchObject({steps: [{type: 'tool', name: 'Bash', result: 'ok'}]});
    });

    it('nests a subagent\'s own work under the Agent call and folds its task state', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Read', request_id: 't0'}),
            msg({kind: 'tool_use', content: 'Agent', request_id: 'a1', payload: {description: 'Scan repo', prompt: 'look around'}}),
            msg({kind: 'task', request_id: 'a1', payload: {event: 'task_started', task_id: 'x1', background: true, subagent_type: 'Explore'}}),
            msg({kind: 'tool_use', content: 'Grep', request_id: 's1', parent: 'a1'}),
            msg({kind: 'tool_result', content: '3 matches', request_id: 's1', parent: 'a1'}),
            msg({kind: 'task', request_id: 'a1', payload: {event: 'task_progress', description: 'Running Grep', last_tool: 'Grep', usage: {total_tokens: 900, tool_uses: 1, duration_ms: 400}}}),
            msg({role: 'assistant', content: 'Found the config loader.', parent: 'a1'}),
            msg({kind: 'tool_result', content: 'Async agent launched', request_id: 'a1'}),
            msg({kind: 'task', request_id: 'a1', payload: {event: 'task_notification', status: 'completed', summary: 'Found it', usage: {total_tokens: 1200, tool_uses: 1, duration_ms: 900}}}),
            msg({role: 'assistant', content: 'Working on it in the background.'}),
        ]);

        expect(blocks.map((b) => b.type)).toEqual(['activity', 'agent', 'assistant']);
        const agent = blocks[1];
        if (agent.type !== 'agent') throw new Error('expected agent');
        expect(agent.call).toMatchObject({name: 'Agent', result: 'Async agent launched'});
        expect(agent.children.map((b) => b.type)).toEqual(['activity', 'assistant']);
        expect(agent.task).toMatchObject({taskId: 'x1', status: 'completed', background: true, subagentType: 'Explore', summary: 'Found it', usage: {tool_uses: 1}});
        expect(agent.task?.activity).toBeUndefined();
        expect(agentReport(agent)).toBe('Found the config loader.');
    });

    it('puts a background command\'s task state on its tool step', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Bash', request_id: 'b1', payload: {command: 'npm test', run_in_background: true}}),
            msg({kind: 'task', request_id: 'b1', payload: {event: 'task_started', task_id: 'bash1', background: true, task_type: 'local_bash'}}),
            msg({kind: 'tool_result', content: 'Command running in background with ID: bash1', request_id: 'b1'}),
            msg({kind: 'task', request_id: 'b1', payload: {event: 'task_updated', status: 'stopped'}}),
        ]);
        expect(blocks[0]).toMatchObject({type: 'activity', steps: [{name: 'Bash', task: {taskId: 'bash1', status: 'stopped', background: true}}]});
    });

    it('starts a new activity block after anything that is not activity', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Read', request_id: 't1'}),
            msg({role: 'assistant', content: 'ok'}),
            msg({kind: 'tool_use', content: 'Edit', request_id: 't2'}),
        ]);
        expect(blocks.map((b) => b.type)).toEqual(['activity', 'assistant', 'activity']);
    });

    it('drops the per-turn Claude session init note but keeps other system notes', () => {
        const blocks = buildTranscript([
            msg({kind: 'system', content: 'claude code session 1234'}),
            msg({kind: 'system', content: 'interrupted; send a message to resume'}),
        ]);
        expect(blocks).toEqual([{type: 'system', message: expect.objectContaining({content: 'interrupted; send a message to resume'})}]);
    });
});

describe('pendingRequestId', () => {
    const blocks = buildTranscript([
        msg({kind: 'approval_request', content: 'Bash', request_id: 'r1'}),
        msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
        msg({kind: 'ask_request', content: 'Which one?', request_id: 'r2'}),
    ]);

    it('is the latest unanswered request while a turn is live', () => {
        expect(pendingRequestId(blocks, true)).toBe('r2');
    });

    it('is nothing once the turn is no longer live', () => {
        expect(pendingRequestId(blocks, false)).toBeUndefined();
    });
});

describe('groupSessionsByFolder', () => {
    it('keeps most-recent-first order between and within folders', () => {
        const s = (id: string, project: string) => ({id, project} as SessionInfo);
        const groups = groupSessionsByFolder([s('a', '/x'), s('b', '/y'), s('c', '/x')]);
        expect(groups.map((g) => [g.path, g.sessions.map((x) => x.id)])).toEqual([['/x', ['a', 'c']], ['/y', ['b']]]);
    });
});

describe('toolSummary', () => {
    it('prefers the field that says what the call did', () => {
        expect(toolSummary({command: 'go test ./...\nsecond line', timeout: 5})).toBe('go test ./...');
        expect(toolSummary({file_path: '/a/b.go', content: 'x'})).toBe('/a/b.go');
    });

    it('falls back to compact JSON, and nothing for empty input', () => {
        expect(toolSummary({foo: 1})).toBe('{"foo":1}');
        expect(toolSummary({})).toBe('');
        expect(toolSummary(undefined)).toBe('');
    });
});

describe('sessionUsage', () => {
    const usage = (payload: object) => msg({kind: 'usage', payload});

    it('totals the turns and keeps the latest turn for model and context', () => {
        const u = sessionUsage([
            msg({role: 'user', content: 'hi'}),
            usage({model: 'tingly/cc', input_tokens: 100, output_tokens: 20, cache_read_tokens: 900, cache_write_tokens: 0, context_tokens: 1000, context_window: 200000}),
            usage({model: 'tingly/cc', input_tokens: 50, output_tokens: 10, cache_read_tokens: 1950, cache_write_tokens: 0, context_tokens: 2000, context_window: 200000}),
        ]);
        expect(u).toMatchObject({input: 150, output: 30, cacheRead: 2850, cacheWrite: 0, latest: {context_tokens: 2000}});
        expect(cacheHitPct(u!)).toBe(95);
    });

    it('is undefined before any turn reached the model', () => {
        expect(sessionUsage([msg({role: 'user', content: 'hi'})])).toBeUndefined();
    });

    it('does not appear in, or split, the rendered transcript', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Read', request_id: 't1'}),
            usage({input_tokens: 1, output_tokens: 1, cache_read_tokens: 0, cache_write_tokens: 0, context_tokens: 1}),
            msg({kind: 'tool_use', content: 'Edit', request_id: 't2'}),
        ]);
        expect(blocks.map((b) => b.type)).toEqual(['activity']);
    });
});

describe('formatTokens', () => {
    it('keeps it short', () => {
        expect(formatTokens(950)).toBe('950');
        expect(formatTokens(12_345)).toBe('12.3k');
        expect(formatTokens(1_234_567)).toBe('1.2M');
    });
});

describe('backgroundTasks', () => {
    const started = (call: string, id: string, type: string, description: string) =>
        msg({kind: 'task', request_id: call, payload: {event: 'task_started', task_id: id, task_type: type, background: true, description}});

    it('lists background work, running first, and settles what its process took down', () => {
        const rows = backgroundTasks([
            started('c1', 't1', 'local_bash', 'Old build'),
            msg({kind: 'task', request_id: 'c1', payload: {event: 'task_notification', status: 'completed'}}),
            started('c2', 't2', 'local_agent', 'Review'),
            started('c3', 't3', 'local_bash', 'Dev server'),
            msg({kind: 'task', request_id: 'c3', payload: {event: 'output_file', task_id: 't3', output_file: '/tmp/x/tasks/t3.output'}}),
            // A foreground subagent is not background work.
            msg({kind: 'task', request_id: 'c4', payload: {event: 'task_started', task_id: 't4', background: false}}),
        ], [{task_id: 't3', task_type: 'local_bash', description: 'Dev server'}]);

        expect(rows.map((r) => [r.taskId, r.status, r.ended])).toEqual([
            ['t3', 'running', false],
            ['t2', 'running', true],
            ['t1', 'completed', false],
        ]);
        expect(rows[0].outputFile).toBe('/tmp/x/tasks/t3.output');
    });
});
