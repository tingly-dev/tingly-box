import type { ComponentProps } from 'react';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ThreadEvent } from '../../client';
import { ConversationPane } from './ConversationPane';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ fetch: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div data-testid="composer" /> }));
vi.mock('../../client', async (orig) => ({ ...(await orig<typeof import('../../client')>()), apiFetch: m.fetch }));

const PNG = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0]);
let blobs: Blob[];
beforeEach(() => {
    localStorage.clear();
    blobs = [];
    URL.createObjectURL = vi.fn((b: Blob) => { blobs.push(b); return `blob:${blobs.length}`; });
    URL.revokeObjectURL = vi.fn();
    m.fetch.mockImplementation(() => Promise.resolve(new Response(PNG)));
});
afterEach(cleanup);

let n = 0;
const sent = (name: string, caption = ''): ThreadEvent => ({
    id: `file-${++n}`, kind: 'file', content: caption, thread_id: 'coordinator', created_at: '2026-11-20T10:00:00Z', updated_at: '',
    payload: JSON.stringify({ attachments: [{ name, path: `outputs/${name}`, type: 'image/png', size: 12 }] }),
});
const props = (messages: ThreadEvent[]): ComponentProps<typeof ConversationPane> => ({
    projectId: 'p', threadId: 'coordinator', messages, threads: new Map(), busy: false, disabled: false,
    onSend: vi.fn().mockResolvedValue(true), onOpenThread: vi.fn(), onStartSuggestions: vi.fn().mockResolvedValue(undefined), onChooseDecision: vi.fn().mockResolvedValue(undefined),
});
const show = (messages: ThreadEvent[]) => render(<MemoryRouter><ProjectsHostProvider host={{ paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: () => '/folders' } }}><ConversationPane {...props(messages)} /></ProjectsHostProvider></MemoryRouter>);

describe('files a thread sent into the conversation', () => {
    it('shows consecutive files as one row of thumbnails, each caption only on its file', async () => {
        show([sent('a.png', 'The diagram'), sent('b.png'), sent('c.png', 'The result')]);
        const row = await screen.findByRole('group', { name: 'projects.transcript.filesSent' });
        expect(within(row).getAllByRole('button', { name: /\.png$/ })).toHaveLength(3);
        expect(screen.getAllByRole('group', { name: 'projects.transcript.filesSent' })).toHaveLength(1);
        // The caption is the thumbnail's alt, not a message.
        await waitFor(() => expect(within(row).getByRole('img', { name: 'The diagram' })).toBeInTheDocument());
        expect(screen.queryByText('The diagram')).toBeNull();
        expect(screen.queryByText('The result')).toBeNull();
    });

    it('shows a single file as its file, its caption only as the alt', async () => {
        show([sent('a.png', 'The diagram')]);
        const thumb = await screen.findByRole('button', { name: 'a.png' });
        expect(screen.queryByRole('group', { name: 'projects.transcript.filesSent' })).toBeNull();
        await waitFor(() => expect(within(thumb).getByRole('img', { name: 'The diagram' })).toBeInTheDocument());
        expect(screen.queryByText('The diagram')).toBeNull();
    });

    it('steps through every image of the conversation from a preview, across messages', async () => {
        show([
            sent('a.png'),
            { id: 'say', kind: 'assistant', content: 'Here is more', thread_id: 'coordinator', created_at: '', updated_at: '' },
            sent('b.png'),
            sent('c.png'),
        ]);
        const first = await screen.findByRole('button', { name: 'a.png' });
        await waitFor(() => expect(within(first).getByRole('img')).toBeInTheDocument());
        fireEvent.click(first);
        const dialog = await screen.findByRole('dialog');
        await within(dialog).findByRole('img', { name: 'a.png' });
        expect(within(dialog).getByText('1 / 3')).toBeInTheDocument();
        fireEvent.keyDown(dialog, { key: 'ArrowRight' });
        await within(dialog).findByRole('img', { name: 'b.png' });
        fireEvent.keyDown(dialog, { key: 'ArrowDown' });
        await within(dialog).findByRole('img', { name: 'c.png' });
        expect(within(dialog).getByText('3 / 3')).toBeInTheDocument();
        fireEvent.click(within(dialog).getByRole('button', { name: 'library.previousImage' }));
        await within(dialog).findByRole('img', { name: 'b.png' });
        expect(within(dialog).getByText('2 / 3')).toBeInTheDocument();
    });
});
