import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {createMemoryDraftStorage} from './context';
import Transcript from './Transcript';
import type {TranscriptEntry} from './model';
import {buildTranscript} from './model';

const {writeToClipboard} = vi.hoisted(() => ({writeToClipboard: vi.fn(async (_text: string) => {})}));
vi.mock('@tingly/ui/utils/clipboard', async (importOriginal) => ({...(await importOriginal<object>()), writeToClipboard}));
vi.mock('react-i18next', () => ({useTranslation: () => ({i18n: {language: 'en'}, t: (_key: string, opts?: {defaultValue?: string}) => opts?.defaultValue ?? _key})}));

const say = (role: 'user' | 'assistant', content: string, extra: Partial<TranscriptEntry> = {}): TranscriptEntry =>
    ({role, content, timestamp: '2026-11-19T08:00:00Z', ...extra});

const view = (entries: TranscriptEntry[]) =>
    render(<Transcript blocks={buildTranscript(entries)} working={false} onRespond={vi.fn()} drafts={createMemoryDraftStorage()}/>);

beforeEach(() => writeToClipboard.mockClear());
afterEach(cleanup);

describe('message copy action', () => {
    it('puts one copy button on each user and assistant message, and none on thinking, tools or system notes', () => {
        view([
            say('user', 'hello'),
            {kind: 'thinking', content: 'let me think', timestamp: '2026-11-19T08:00:01Z'},
            {kind: 'tool_use', content: 'Bash', request_id: 't1', payload: {command: 'ls'}, timestamp: '2026-11-19T08:00:02Z'},
            {kind: 'tool_result', content: 'a.txt', request_id: 't1', timestamp: '2026-11-19T08:00:03Z'},
            say('assistant', 'hi there'),
            {kind: 'system', content: 'session note', timestamp: '2026-11-19T08:00:04Z'},
        ]);
        expect(screen.getAllByRole('button', {name: 'Copy message'})).toHaveLength(2);
    });

    it('copies the raw Markdown exactly — not the rendered text, host context, attachments or time', async () => {
        const markdown = '## Result\n\nUse **`go test`** and:\n\n```go\nfmt.Println("x")\n```\n';
        view([
            say('user', 'run the tests', {context: 'Decision card · Option B', author: 'Coordinator'}),
            say('assistant', markdown),
        ]);
        const buttons = screen.getAllByRole('button', {name: 'Copy message'});
        fireEvent.click(buttons[0]);
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith('run the tests'));
        fireEvent.click(buttons[1]);
        await waitFor(() => expect(writeToClipboard).toHaveBeenLastCalledWith(markdown));
        expect(writeToClipboard.mock.calls.at(-1)?.[0]).not.toContain('2026');
        expect(writeToClipboard.mock.calls.at(-1)?.[0]).not.toContain('Decision card');
    });

    it('acknowledges a successful copy with a check and a “Copied” action', async () => {
        view([say('assistant', 'done')]);
        fireEvent.click(screen.getByRole('button', {name: 'Copy message'}));
        expect(await screen.findByRole('button', {name: 'Copied'})).toBeTruthy();
        expect(screen.getByRole('button', {name: 'Copied'}).getAttribute('data-copy-state')).toBe('copied');
    });

    it('says so when the clipboard refuses', async () => {
        writeToClipboard.mockRejectedValueOnce(new Error('denied'));
        view([say('user', 'hello')]);
        fireEvent.click(screen.getByRole('button', {name: 'Copy message'}));
        expect(await screen.findByRole('button', {name: 'Could not copy'})).toBeTruthy();
        expect(screen.getByRole('button', {name: 'Could not copy'}).getAttribute('data-copy-state')).toBe('failed');
    });

    it('is a native button a keyboard can focus and activate', async () => {
        view([say('assistant', 'done')]);
        const button = screen.getByRole('button', {name: 'Copy message'}) as HTMLButtonElement;
        expect(button.tagName).toBe('BUTTON');
        button.focus();
        expect(document.activeElement).toBe(button);
        fireEvent.click(button);
        await waitFor(() => expect(writeToClipboard).toHaveBeenCalledWith('done'));
    });
});
