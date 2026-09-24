import {describe, expect, it} from 'vitest';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {buildTranscript, groupSessionsByFolder, pendingRequestId, toolSummary} from './deskUtils';

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
