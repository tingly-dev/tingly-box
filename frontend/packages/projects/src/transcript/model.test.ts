import {describe, expect, it} from 'vitest';
import type {TranscriptEntry} from './model';
import {agentReport, approvalSubject, buildTranscript, firstNewBlock, formatTokens, latestTodos, pendingRequestId, requestOutcome, toolSummary, workingState} from './model';

const msg = (m: Partial<TranscriptEntry>): TranscriptEntry => ({content: '', timestamp: '2026-01-01T00:00:00Z', ...m});

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
            {type: 'thinking', text: 'look at main.go', timestamp: '2026-01-01T00:00:00Z'},
            {type: 'tool', id: 't1', name: 'Read', input: {file_path: 'main.go'}, result: 'package main', isError: false, task: undefined, timestamp: '2026-01-01T00:00:00Z'},
            {type: 'tool', id: 't2', name: 'Bash', input: {command: 'go test'}, result: 'FAIL', isError: true, task: undefined, timestamp: '2026-01-01T00:00:00Z'},
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

    it('leaves out steps of a call that is not loaded, and keeps output under a call that is not a subagent', () => {
        const blocks = buildTranscript([
            msg({role: 'user', content: 'go'}),
            msg({kind: 'tool_use', content: 'Grep', request_id: 's9', parent: 'gone'}),
            msg({kind: 'tool_use', content: 'Bash', request_id: 'b1'}),
            msg({role: 'assistant', content: 'from under Bash', parent: 'b1'}),
        ]);
        expect(blocks.map((b) => b.type)).toEqual(['user', 'activity', 'assistant']);
        expect(JSON.stringify(blocks)).not.toContain('s9');
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
            msg({kind: 'task', request_id: 'b1', payload: {event: 'output_snapshot', task_id: 'bash1', output: 'PASS\n', truncated: false}}),
        ]);
        expect(blocks[0]).toMatchObject({steps: [{task: {outputSnapshot: {content: 'PASS\n', truncated: false}}}]});
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

describe('usage entries', () => {
    const usage = (payload: object) => msg({kind: 'usage', payload});

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

describe('approvals that belong to a call', () => {
    it('links an approval to the call right before it, by the same tool', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Bash', request_id: 't1', payload: {command: 'go test'}}),
            msg({kind: 'usage', payload: {input_tokens: 1, output_tokens: 1, cache_read_tokens: 0, cache_write_tokens: 0, context_tokens: 1}}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: {command: 'go test'}}),
            msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
            msg({kind: 'tool_result', content: 'ok', request_id: 't1'}),
        ]);
        const [activity, request] = blocks;
        if (activity.type !== 'activity' || activity.steps[0].type !== 'tool' || request.type !== 'request') throw new Error('unexpected blocks');
        expect(request.toolId).toBe('t1');
        expect(activity.steps[0].request).toBe(request);
    });

    it('leaves an approval alone when the call before it is another tool or already ran', () => {
        const blocks = buildTranscript([
            msg({kind: 'tool_use', content: 'Read', request_id: 't1'}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1'}),
            msg({kind: 'tool_use', content: 'Bash', request_id: 't2'}),
            msg({kind: 'tool_result', content: 'ok', request_id: 't2'}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r2'}),
            msg({kind: 'ask_request', content: 'Which branch?', request_id: 'q1'}),
        ]);
        expect(blocks.filter((b) => b.type === 'request').map((b) => b.type === 'request' && b.toolId)).toEqual([undefined, undefined, undefined]);
    });
});

describe('requestOutcome', () => {
    const request = (kind: string, answer?: string) => buildTranscript([
        msg({kind, content: 'Bash', request_id: 'r1'}),
        ...(answer === undefined ? [] : [msg({kind: kind.replace('request', 'response'), content: answer, request_id: 'r1'})]),
    ])[0] as Extract<ReturnType<typeof buildTranscript>[number], {type: 'request'}>;

    it('reads the engine\'s answer', () => {
        expect(requestOutcome(request('approval_request', 'approved'), false)).toBe('allowed');
        expect(requestOutcome(request('approval_request', 'denied'), false)).toBe('denied');
        expect(requestOutcome(request('ask_request', 'main'), false)).toBe('answered');
        expect(requestOutcome(request('ask_request', ''), false)).toBe('declined');
        expect(requestOutcome(request('approval_request'), false)).toBe('unanswered');
        expect(requestOutcome(request('approval_request'), true)).toBe('pending');
    });
});

describe('approvalSubject', () => {
    it('is the concrete thing being approved', () => {
        expect(approvalSubject('Bash', {command: 'rm -rf dist\nls', description: 'Clean'})).toEqual({kind: 'command', text: 'rm -rf dist\nls'});
        expect(approvalSubject('Edit', {file_path: '/a/b.go', old_string: 'x'})).toEqual({kind: 'path', text: '/a/b.go'});
        expect(approvalSubject('NotebookEdit', {notebook_path: '/a/n.ipynb'})).toEqual({kind: 'path', text: '/a/n.ipynb'});
        expect(approvalSubject('WebFetch', {url: 'https://x.y', prompt: 'read it'})).toEqual({kind: 'url', text: 'https://x.y'});
        expect(approvalSubject('Grep', {pattern: 'TODO'})).toEqual({kind: 'summary', text: 'TODO'});
    });
});

describe('firstNewBlock', () => {
    const blocks = buildTranscript([
        msg({role: 'user', content: 'a', timestamp: '2026-01-01T00:00:00Z'}),
        msg({kind: 'tool_use', content: 'Read', request_id: 't1', timestamp: '2026-01-01T00:01:00Z'}),
        msg({role: 'assistant', content: 'b', timestamp: '2026-01-01T00:02:00Z'}),
    ]);

    it('is the first block that began after the mark', () => {
        expect(firstNewBlock(blocks, '2026-01-01T00:00:30Z')).toBe(1);
        expect(firstNewBlock(blocks, '2026-01-01T00:01:00Z')).toBe(2);
    });

    it('is none without a mark, with nothing new, or with everything new', () => {
        expect(firstNewBlock(blocks)).toBe(-1);
        expect(firstNewBlock(blocks, '2026-01-01T00:02:00Z')).toBe(-1);
        expect(firstNewBlock(blocks, '2025-12-31T00:00:00Z')).toBe(-1);
        expect(firstNewBlock(blocks, 'not a time')).toBe(-1);
    });
});

describe('workingState', () => {
    const user = msg({role: 'user', content: 'go', timestamp: '2026-01-01T00:00:10Z'});

    it('reads the verb from the last call and the start from the last ask', () => {
        const at = (...rest: TranscriptEntry[]) => workingState(buildTranscript([user, ...rest]));
        expect(at()).toEqual({verb: 'thinking', since: '2026-01-01T00:00:10Z'});
        expect(at(msg({kind: 'tool_use', content: 'Bash', request_id: 't1'})).verb).toBe('running');
        expect(at(msg({kind: 'tool_use', content: 'Grep', request_id: 't1'})).verb).toBe('reading');
        expect(at(msg({kind: 'tool_use', content: 'mcp__x__y', request_id: 't1'})).verb).toBe('working');
        expect(at(msg({kind: 'tool_use', content: 'Edit', request_id: 't1'}), msg({kind: 'tool_result', content: 'ok', request_id: 't1'})).verb).toBe('thinking');
        expect(at(msg({kind: 'thinking', content: 'hmm'})).verb).toBe('thinking');
    });
});

describe('latestTodos', () => {
    const write = (id: string, todos: unknown) => msg({kind: 'tool_use', content: 'TodoWrite', request_id: id, payload: {todos}});

    it('reads the last TodoWrite, which rewrites the whole list', () => {
        const blocks = buildTranscript([
            msg({role: 'user', content: 'go'}),
            write('a', [{content: 'one', status: 'pending'}, {content: 'two', status: 'pending'}]),
            write('b', [{content: 'one', status: 'completed'}, {content: 'two', activeForm: 'Doing two', status: 'in_progress'}, {content: '', status: 'pending'}]),
        ]);
        expect(latestTodos(blocks)).toEqual([
            {content: 'one', activeForm: undefined, status: 'completed'},
            {content: 'two', activeForm: 'Doing two', status: 'in_progress'},
        ]);
    });

    it('is undefined without a usable list', () => {
        expect(latestTodos(buildTranscript([msg({role: 'user', content: 'go'}), write('a', 'nope')]))).toBeUndefined();
        expect(latestTodos(buildTranscript([msg({role: 'user', content: 'go'})]))).toBeUndefined();
    });
});
