import {describe, expect, it} from 'vitest';
import {messageCopyText} from './copyText';
import type {TranscriptBlock, TranscriptEntry} from './model';

const entry = (m: Partial<TranscriptEntry>): TranscriptEntry => ({content: '', timestamp: '2026-01-01T00:00:00Z', ...m});
const message = (role: 'user' | 'assistant', m: Partial<TranscriptEntry> = {}): TranscriptBlock => ({type: role, message: entry({role, ...m})});

describe('messageCopyText', () => {
    it('returns a user message exactly as written', () => {
        const text = 'line one\n  indented\n\n- not a list to us, just text';
        expect(messageCopyText(message('user', {content: text}))).toBe(text);
    });

    it('returns an assistant reply as its raw Markdown, not the rendered text', () => {
        const content = '## Plan\n\nUse **`go test`** and:\n\n```go\nfmt.Println("x")\n```\n';
        expect(messageCopyText(message('assistant', {content}))).toBe(content);
    });

    it('leaves out the host context line, author, timestamp and attachments', () => {
        const block = message('user', {
            content: 'just the words',
            context: 'Decision card · Option B',
            author: 'Coordinator',
            timestamp: '2026-03-04T05:06:07Z',
            attachments: [{name: 'a.png', path: '/a.png', type: 'image/png', size: 3}],
        });
        expect(messageCopyText(block)).toBe('just the words');
    });

    it('has nothing to copy for an empty or whitespace-only message', () => {
        expect(messageCopyText(message('user', {content: ''}))).toBeUndefined();
        expect(messageCopyText(message('assistant', {content: '  \n\t '}))).toBeUndefined();
    });

    it('has nothing to copy for files the agent sent (their content is a caption)', () => {
        const block = message('assistant', {
            content: 'Here is the report',
            attachments: [{name: 'r.pdf', path: '/r.pdf', type: 'application/pdf', size: 1}],
        });
        expect(messageCopyText(block)).toBeUndefined();
    });

    it('is only for user and assistant messages, not thinking, tools, requests or system notes', () => {
        const others: TranscriptBlock[] = [
            {type: 'activity', steps: [{type: 'thinking', text: 'hmm'}, {type: 'tool', id: 't1', name: 'Bash', input: {command: 'ls'}, result: 'ok', isError: false}]},
            {type: 'request', message: entry({kind: 'approval_request', content: 'Bash'})},
            {type: 'error', message: entry({kind: 'error', content: 'boom'})},
            {type: 'command', message: entry({kind: 'command', content: 'compact', payload: {name: 'compact', state: 'ok', summary: 'sum'}})},
            {type: 'system', message: entry({kind: 'system', content: 'note'})},
            {type: 'custom', message: entry({kind: 'thread_update', content: 'card'})},
            {type: 'agent', call: {type: 'tool', id: 'a1', name: 'Agent', input: {}, isError: false}, children: []},
        ];
        for (const block of others) expect(messageCopyText(block)).toBeUndefined();
    });
});
