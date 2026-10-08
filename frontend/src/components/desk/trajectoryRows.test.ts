import {describe, expect, it} from 'vitest';
import type {MessageInfo} from '@/services/deskApi';
import {buildTranscript} from './deskUtils';
import {matchesFilter, plainLine, trajectoryRows, trajectorySummary} from './trajectoryRows';

const msg = (m: Partial<MessageInfo>): MessageInfo => ({content: '', timestamp: '2026-01-01T00:00:00Z', ...m} as MessageInfo);

const rowsOf = (messages: MessageInfo[], live = false) => trajectoryRows(buildTranscript(messages), live);

describe('trajectoryRows', () => {
    it('puts one action per row and leaves thinking out', () => {
        const rows = rowsOf([
            msg({role: 'user', content: 'fix the bug\nplease', timestamp: '2026-01-01T10:00:00Z'}),
            msg({kind: 'thinking', content: 'look at main.go'}),
            msg({kind: 'tool_use', content: 'Read', request_id: 't1', payload: {file_path: 'main.go'}}),
            msg({kind: 'tool_use', content: 'Bash', request_id: 't2', payload: {command: 'go test ./...'}}),
            msg({kind: 'tool_result', content: 'package main', request_id: 't1', payload: {is_error: false}}),
            msg({kind: 'tool_result', content: 'FAIL', request_id: 't2', payload: {is_error: true}}),
            msg({role: 'assistant', content: '## Found it\n\nThe **loop** was off by one.'}),
        ]);

        expect(rows).toEqual([
            {kind: 'user', turn: 1, depth: 0, anchor: {block: 0}, title: 'fix the bug', time: '2026-01-01T10:00:00Z'},
            {kind: 'tool', turn: 1, depth: 0, anchor: {call: 't1'}, title: 'Read', detail: 'main.go', failed: false, file: undefined, command: false},
            {kind: 'tool', turn: 1, depth: 0, anchor: {call: 't2'}, title: 'Bash', detail: 'go test ./...', failed: true, file: undefined, command: true},
            {kind: 'reply', turn: 1, depth: 0, anchor: {block: 2}, title: 'Found it'},
        ]);
    });

    it('counts turns by user messages and shows answered approvals with their outcome', () => {
        const rows = rowsOf([
            msg({role: 'user', content: 'one'}),
            msg({role: 'assistant', content: 'ok'}),
            msg({role: 'user', content: 'two'}),
            msg({kind: 'approval_request', content: 'Bash', request_id: 'r1', payload: {command: 'rm -rf dist'}}),
            msg({kind: 'approval_response', content: 'approved', request_id: 'r1'}),
            msg({kind: 'error', content: 'turn failed: exit 1'}),
        ]);

        expect(rows.map((r) => [r.kind, r.turn])).toEqual([['user', 1], ['reply', 1], ['user', 2], ['approval', 2], ['error', 2]]);
        expect(rows[3]).toMatchObject({title: 'Bash', detail: 'rm -rf dist', outcome: 'approved', anchor: {request: 'r1'}});
        expect(rows[4]).toMatchObject({failed: true});
    });

    it('nests a subagent\'s steps under it, anchored to its card', () => {
        const rows = rowsOf([
            msg({kind: 'tool_use', content: 'Agent', request_id: 'a1', payload: {description: 'Scan repo', subagent_type: 'Explore'}}),
            msg({kind: 'tool_use', content: 'Grep', request_id: 's1', parent: 'a1', payload: {pattern: 'TODO'}}),
            msg({kind: 'tool_result', content: '3 matches', request_id: 's1', parent: 'a1'}),
            msg({role: 'assistant', content: 'Found three.', parent: 'a1'}),
            msg({kind: 'tool_result', content: 'done', request_id: 'a1'}),
        ]);

        expect(rows).toMatchObject([
            {kind: 'agent', depth: 0, title: 'Explore', detail: 'Scan repo', status: 'completed', anchor: {call: 'a1'}},
            {kind: 'tool', depth: 1, title: 'Grep', detail: 'TODO', anchor: {call: 'a1'}},
            {kind: 'reply', depth: 1, title: 'Found three.', anchor: {call: 'a1'}},
        ]);
    });

    it('reads a subagent without a result as running only while the turn runs', () => {
        const messages = [msg({kind: 'tool_use', content: 'Agent', request_id: 'a1', payload: {description: 'Scan'}})];
        expect(rowsOf(messages, true)[0]).toMatchObject({status: 'running', failed: false});
        expect(rowsOf(messages, false)[0]).toMatchObject({status: 'stopped'});
    });
});

describe('notes and replies', () => {
    const turn = [
        msg({role: 'user', content: 'fix it'}),
        msg({role: 'assistant', content: 'Let me run the tests first.'}),
        msg({kind: 'tool_use', content: 'Bash', request_id: 'b1', payload: {command: 'make test'}}),
        msg({kind: 'tool_result', content: 'ok', request_id: 'b1'}),
        msg({role: 'assistant', content: 'All green now.'}),
    ];

    it('calls the last text of a turn its reply and the text between steps notes', () => {
        const rows = rowsOf([...turn, msg({role: 'user', content: 'thanks'}), msg({role: 'assistant', content: 'Anytime.'})]);
        expect(rows.filter((r) => r.kind === 'note' || r.kind === 'reply').map((r) => [r.kind, r.title])).toEqual([
            ['note', 'Let me run the tests first.'],
            ['reply', 'All green now.'],
            ['reply', 'Anytime.'],
        ]);
    });

    it('does not call text a reply while the running turn still has work after it', () => {
        const running = rowsOf(turn.slice(0, 3), true);
        expect(running.find((r) => r.title === 'Let me run the tests first.')?.kind).toBe('note');
        const answered = rowsOf(turn, true);
        expect(answered.find((r) => r.title === 'All green now.')?.kind).toBe('reply');
    });
});

describe('trajectorySummary', () => {
    it('counts distinct edited files, commands, failures and requests', () => {
        const rows = rowsOf([
            msg({role: 'user', content: 'go'}),
            msg({kind: 'tool_use', content: 'Edit', request_id: 'e1', payload: {file_path: 'a.ts'}}),
            msg({kind: 'tool_use', content: 'Write', request_id: 'e2', payload: {file_path: 'b.ts'}}),
            msg({kind: 'tool_use', content: 'Edit', request_id: 'e3', payload: {file_path: 'a.ts'}}),
            msg({kind: 'tool_use', content: 'Bash', request_id: 'b1', payload: {command: 'make'}}),
            msg({kind: 'tool_result', content: 'boom', request_id: 'b1', payload: {is_error: true}}),
            msg({kind: 'ask_request', content: 'Which branch?', request_id: 'q1'}),
        ]);

        expect(trajectorySummary(rows)).toEqual({turns: 1, files: ['a.ts', 'b.ts'], commands: 1, failures: 1, requests: 1});
        expect(rows.filter((r) => matchesFilter(r, 'files')).map((r) => r.file)).toEqual(['a.ts', 'b.ts', 'a.ts']);
    });
});

describe('plainLine', () => {
    it('drops markup that only reads rendered', () => {
        expect(plainLine('\n- Use [the docs](https://x.y) and `make`')).toBe('Use the docs and make');
    });
});
