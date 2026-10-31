import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import type { Thread, ThreadEvent } from '../../client';
import { ConversationPane } from './ConversationPane';
import { showManagerMessage } from './messageDetail';
import { readLocal, writeLocal } from '../../utils/storage';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div> }));
vi.mock('../attachments', () => ({ AttachmentChips: ({ items }: { items: { name: string }[] }) => <div>{items.map((item) => <button key={item.name}>{item.name}</button>)}</div>, uploadAttachments: vi.fn() }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: ({ onSend, context, onModelChange }: { onSend: (text: string, files: []) => Promise<boolean>; context?: React.ReactNode; onModelChange?: (model: string) => void }) => <div>{context}<button onClick={() => onModelChange?.('next-model')}>Switch model</button><button onClick={() => void onSend('New message', [])}>Send message</button></div> }));

beforeEach(() => localStorage.clear());
afterEach(cleanup);
const event = (id: string, kind: string, content: string, extra: Partial<ThreadEvent> = {}): ThreadEvent => ({
    id, kind, content, thread_id: 'main', created_at: '', updated_at: '', ...extra,
});
const messages = [
    event('u', 'user', 'User request', { payload: JSON.stringify({ attachments: [{ name: 'evidence.png', path: 'evidence.png', type: 'image/png', size: 100 }] }) }),
    event('a', 'assistant', 'Manager reply'),
    event('peer', 'assistant', 'Peer message', { source: 'thread' }),
    event('start', 'thread_started', '', { payload: JSON.stringify({ thread_ids: ['worker'] }) }),
    event('report', 'thread_update', 'Thread report', { payload: JSON.stringify({ thread_id: 'worker', title: 'Worker report', kind: 'needs_input', status: 'waiting' }) }),
    event('suggest', 'suggestions', '', { payload: JSON.stringify({ suggestions: [{ title: 'Proposed task', task: 'Do it' }] }) }),
    event('err', 'error', 'Thread failed'),
];
const worker = { id: 'worker', title: 'Worker title', status: 'running', branch: '', commits_ahead: 0, files_changed: 0 } as Thread;
const props = () => ({
    projectId: 'project', threadId: 'main', messages, threads: new Map([['worker', worker]]), busy: false, disabled: false,
    onSend: vi.fn().mockResolvedValue(true), onOpenThread: vi.fn(), onStartSuggestions: vi.fn().mockResolvedValue(undefined), onChooseDecision: vi.fn().mockResolvedValue(undefined),
    onModelChange: vi.fn(), composerContext: <button>Switch control</button>,
});
const show = (p = props()) => render(<MemoryRouter><ConversationPane {...p} /></MemoryRouter>);

describe('manager message detail', () => {
    it('hides finished notifications, not ended thread cards, and restores them in Detailed', () => {
        const p = props();
        p.threads.set('worker', { ...worker, status: 'resolved' });
        p.messages = [...messages, event('finish', 'thread_update', 'Finished report body', {
            payload: JSON.stringify({ thread_id: 'worker', title: 'Finished notification', kind: 'finished', status: 'resolved' }),
        })];
        const original = JSON.stringify(p.messages);
        show(p);
        expect(screen.getByText('Finished notification')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        expect(screen.queryByText('Finished notification')).toBeNull();
        expect(screen.getByText('Worker report')).toBeInTheDocument();
        expect(screen.getByText('User request')).toBeInTheDocument();
        expect(screen.getByText('Manager reply')).toBeInTheDocument();
        fireEvent.click(screen.getByText('Worker title'));
        expect(p.onOpenThread).toHaveBeenCalledWith('worker');
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.detailed' }));
        fireEvent.click(screen.getByText('Finished notification'));
        expect(screen.getByText('Finished report body')).toBeInTheDocument();
        expect(JSON.stringify(p.messages)).toBe(original);
    });
    it.each(['needs_input', 'failed', 'unknown'])('keeps %s notifications and malformed payloads eligible', (kind) => {
        expect(showManagerMessage(event('update', 'thread_update', '', { payload: JSON.stringify({ kind }) }), 'concise')).toBe(true);
        expect(showManagerMessage(event('broken', 'thread_update', '', { payload: '{' }), 'concise')).toBe(true);
    });
    it.each(['user', 'assistant'])('filters cross-thread %s text by provenance, not role', (kind) => {
        for (const source of [undefined, '', 'im', 'system']) {
            expect(showManagerMessage(event('local', kind, 'Keep', { source }), 'concise')).toBe(true);
        }
        const peer = event('peer', kind, 'Peer', { source: 'thread', source_ref: 'worker' });
        expect(showManagerMessage(peer, 'concise')).toBe(false);
        expect(showManagerMessage(peer, 'detailed')).toBe(true);
    });
    it.each(['ready', 'resolved', 'failed'])('retains ended %s thread cards, report bodies and navigation in concise mode', (status) => {
        writeLocal('conversation.messageDetail', 'concise');
        const p = props();
        p.threads.set('worker', { ...worker, status });
        show(p);
        expect(screen.getByText('User request')).toBeInTheDocument();
        fireEvent.click(screen.getByText('Worker title'));
        expect(p.onOpenThread).toHaveBeenCalledWith('worker');
        fireEvent.click(screen.getByText('Worker report'));
        expect(screen.getByText('Thread report')).toBeInTheDocument();
    });
    it.each(['thread_started', 'thread_update', 'suggestions', 'switch', 'approval', 'ask', 'error', 'future_card'])('does not pre-filter structured %s entries, regardless of source', (kind) => {
        expect(showManagerMessage(event('card', kind, 'Card', { source: 'thread' }), 'concise')).toBe(true);
    });
    it('defaults to the complete view and concise keeps manager speech and every card', async () => {
        const p = props();
        const original = JSON.stringify(p.messages);
        show(p);
        expect(screen.getByText('User request')).toBeInTheDocument();
        expect(screen.getByText('Manager reply')).toBeInTheDocument();
        expect(screen.getByText('Worker title')).toBeInTheDocument();
        expect(screen.getByText('Worker report')).toBeInTheDocument();
        expect(screen.getByText('Proposed task')).toBeInTheDocument();
        expect(screen.getByText('Thread failed')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        expect(screen.getByText('Manager reply')).toBeInTheDocument();
        expect(screen.getByText('User request')).toBeInTheDocument();
        expect(screen.queryByText('Peer message')).toBeNull();
        for (const text of ['Worker title', 'Worker report', 'Proposed task', 'Thread failed']) expect(screen.getByText(text)).toBeInTheDocument();
        fireEvent.click(screen.getByText('Worker title'));
        expect(p.onOpenThread).toHaveBeenCalledWith('worker');
        fireEvent.click(screen.getByText('Worker report'));
        expect(screen.getByText('Thread report')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'conversation.openThread' }));
        expect(p.onOpenThread).toHaveBeenCalledTimes(2);
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'conversation.start' })));
        expect(p.onStartSuggestions).toHaveBeenCalledWith('suggest', [0]);
        expect(screen.getByRole('button', { name: 'evidence.png' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Switch control' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Switch model' }));
        expect(p.onModelChange).toHaveBeenCalledWith('next-model');
        expect(readLocal('conversation.messageDetail')).toBe('concise');
        expect(JSON.stringify(p.messages)).toBe(original);
        await act(async () => fireEvent.click(screen.getByText('Send message')));
        expect(p.onSend).toHaveBeenCalledWith('New message', [], undefined);
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.detailed' }));
        expect(screen.getByText('Worker report')).toBeInTheDocument();
    });

    it('retains every multi-sentence manager reply in order across cards, updates and detail switches', () => {
        const replies = ['First manager reply. Its second sentence remains.', 'Second manager reply. Still complete.', 'Third manager reply. Nothing is collapsed.'];
        const p = props();
        const initial = [event('a1', 'assistant', replies[0]), messages[3], event('a2', 'assistant', replies[1]), messages[4], messages[2]];
        const view = render(<MemoryRouter><ConversationPane {...p} messages={initial} /></MemoryRouter>);
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        const updated = [...initial, event('a3', 'assistant', replies[2])];
        view.rerender(<MemoryRouter><ConversationPane {...p} messages={updated} /></MemoryRouter>);
        const assertReplies = () => {
            const nodes = replies.map((reply) => screen.getByText(reply));
            expect(nodes[0].compareDocumentPosition(screen.getByText('Worker title')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
            expect(screen.getByText('Worker title').compareDocumentPosition(nodes[1]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
            expect(nodes[1].compareDocumentPosition(screen.getByText('Worker report')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
            expect(screen.getByText('Worker report').compareDocumentPosition(nodes[2]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
        };
        assertReplies();
        expect(screen.queryByText('Peer message')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.detailed' }));
        assertReplies();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        assertReplies();
        view.unmount();
        render(<MemoryRouter><ConversationPane {...p} messages={updated} /></MemoryRouter>);
        assertReplies();
    });

    it('restores concise after remount and migrates old hidden-report settings', () => {
        writeLocal('conversation.threadReports', '0');
        const first = show();
        expect(screen.getByText('Worker report')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'messageDetail.concise' })).toHaveAttribute('aria-pressed', 'true');
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.detailed' }));
        first.unmount();
        const second = show();
        expect(screen.getByText('Worker report')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'messageDetail.concise' }));
        second.unmount();
        show();
        expect(screen.getByText('Worker title')).toBeInTheDocument();
        expect(screen.queryByText('Peer message')).toBeNull();
        expect(screen.getByText('Manager reply')).toBeInTheDocument();
    });
});
