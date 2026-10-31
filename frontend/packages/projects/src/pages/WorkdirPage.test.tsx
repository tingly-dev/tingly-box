import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import { forgetTranscripts } from '../hooks/useTranscript';
import WorkdirPage from './WorkdirPage';

type Frame = { type: string; data: unknown };
const m = vi.hoisted(() => ({
    get: vi.fn(), del: vi.fn(), post: vi.fn(), put: vi.fn(), folders: [] as unknown[], reload: vi.fn(),
    // The two streams, split the way the real hub splits them.
    listCb: null as null | ((ev: Frame) => void),
    projectUrls: [] as string[],
    projectCb: null as null | ((ev: Frame) => void),
}));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key), i18n: { language: 'en' } }),
}));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get, DELETE: m.del, POST: m.post, PUT: m.put } }));
vi.mock('../hooks/useProjectList', () => ({
    useProjectList: (_on: boolean, onEvent: (ev: Frame) => void) => {
        m.listCb = onEvent;
        return { folders: m.folders, all: m.folders, projects: [], waiting: 0, reload: m.reload };
    },
}));
vi.mock('../hooks/useProjectEvents', () => ({
    useProjectEvents: (id: string, onEvent: (ev: Frame) => void) => {
        m.projectCb = id ? onEvent : null;
        if (id && !m.projectUrls.includes(id)) m.projectUrls.push(id);
    },
}));

const folder = (working: number, waiting: number) => ({
    id: 'f1', name: 'app', path: '/home/me/app', type: 'folder', starred: false,
    threads: { working, waiting, ready: 0, idle: 0, resolved: 0 },
});
const host: ProjectsHost = { paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f' } };

const show = (entry = '/?path=%2Fhome%2Fme%2Fapp') => render(
    <MemoryRouter initialEntries={[entry]}><ProjectsHostProvider host={host}><WorkdirPage /></ProjectsHostProvider></MemoryRouter>,
);

beforeEach(() => {
    m.get.mockImplementation(async (url: string) => ({
        data: url.endsWith('permission-modes') ? { modes: [] } : { threads: [] },
        response: new Response(),
    }));
    m.del.mockResolvedValue({ data: {}, response: new Response() });
});
afterEach(() => {
    cleanup();
    forgetTranscripts();
    for (const f of [m.get, m.del, m.post, m.put, m.reload]) f.mockReset();
});

describe('WorkdirPage removing a folder', () => {
    it('asks first, counting the sessions it would stop', async () => {
        m.folders = [folder(1, 1)];
        show();
        fireEvent.click(await screen.findByRole('button', { name: 'workdir.remove' }));
        expect(await screen.findByText(/workdir\.removeRunning.*"count":2/)).toBeInTheDocument();
        expect(m.del).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'workdir.removeConfirm' }));
        await waitFor(() => expect(m.del).toHaveBeenCalledTimes(1));
    });

    it('removes at once when nothing is running', async () => {
        m.folders = [folder(0, 0)];
        show();
        fireEvent.click(await screen.findByRole('button', { name: 'workdir.remove' }));
        await waitFor(() => expect(m.del).toHaveBeenCalledTimes(1));
        expect(screen.queryByText(/workdir\.removeRunning/)).not.toBeInTheDocument();
    });

    it('keeps Attach disabled in the new-session prompt until a folder is chosen', async () => {
        m.folders = [];
        show('/');
        expect(await screen.findByRole('button', { name: 'composer.attach' })).toBeDisabled();
    });
});

describe('WorkdirPage live transcript', () => {
    const thread = { id: 't1', project_id: 'f1', title: 'fix it', status: 'working', updated_at: '2026-10-01T00:00:00Z' };
    const entry = (id: string, content: string) => ({ id, kind: 'user', content, thread_id: 't1', created_at: '2026-10-01T00:00:00Z' });

    beforeEach(() => {
        m.listCb = null;
        m.projectCb = null;
        m.projectUrls = [];
        m.folders = [folder(1, 0)];
        m.get.mockImplementation(async (url: string) => {
            const data = url.endsWith('permission-modes') ? { modes: [] }
                : url === '/api/v1/threads/{id}' ? { thread, background_tasks: [], running: true }
                    : url === '/api/v1/threads/{id}/events' ? { events: [] }
                        : { threads: [thread] };
            return { data, response: new Response() };
        });
    });

    it('saves thread permissions before continuing and never sends while that save is pending', async () => {
        let finish!: (value: unknown) => void;
        m.put.mockReturnValue(new Promise((r) => { finish = r; }));
        m.post.mockResolvedValue({ data: thread, response: new Response() });
        const oldGet = m.get.getMockImplementation()!;
        m.get.mockImplementation((url: string, options: unknown) => url.endsWith('permission-modes')
            ? Promise.resolve({ data: { modes: ['plan'] }, response: new Response() }) : oldGet(url, options));
        show('/?path=%2Fhome%2Fme%2Fapp&session=t1');
        const permissions = await screen.findByRole('combobox', { name: 'threadSettings.permissions' });
        const input = screen.getByPlaceholderText('session.queuePlaceholder');
        fireEvent.change(input, { target: { value: 'continue' } });
        fireEvent.mouseDown(permissions);
        fireEvent.click(await screen.findByRole('option', { name: /session.mode.plan/ }));
        expect(m.put).toHaveBeenCalledWith('/api/v1/threads/{id}/permission-mode', { params: { path: { id: 't1' } }, body: { permission_mode: 'plan' } });
        fireEvent.keyDown(input, { key: 'Enter' });
        expect(m.post).not.toHaveBeenCalled();
        expect(input).toHaveValue('continue');
        await act(async () => finish({ data: { ...thread, permission_mode: 'plan' }, response: new Response() }));
        fireEvent.keyDown(input, { key: 'Enter', altKey: true });
        await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/messages', { params: { path: { id: 't1' } }, body: { content: 'continue', attachments: [], mode: 'queue' } }));
    });

    // The real hub sends thread_event only on /projects/{id}/events; the
    // all-projects stream (useProjectList) carries thread / project frames.
    it('takes transcript entries from the open folder\'s own stream, not the all-projects one', async () => {
        show('/?path=%2Fhome%2Fme%2Fapp&session=t1');
        await waitFor(() => expect(m.projectCb).not.toBeNull());
        expect(m.projectUrls).toEqual(['f1']);

        // Not delivered by the all-projects stream (and if it were, it is not this page's source).
        act(() => m.listCb?.({ type: 'thread_event', data: entry('e0', 'from the list stream') }));
        expect(screen.queryByText('from the list stream')).not.toBeInTheDocument();

        act(() => m.projectCb?.({ type: 'thread_event', data: entry('e1', 'live hello') }));
        expect(await screen.findByText('live hello')).toBeInTheDocument();
    });
    it('re-reads only the thread, not its transcript, when the live background tasks change', async () => {
        show('/?path=%2Fhome%2Fme%2Fapp&session=t1');
        await waitFor(() => expect(m.projectCb).not.toBeNull());
        const calls = (url: string) => m.get.mock.calls.filter(([u]) => u === url).length;
        await waitFor(() => expect(calls('/api/v1/threads/{id}/events')).toBe(1));
        const thread0 = calls('/api/v1/threads/{id}');

        act(() => m.projectCb?.({ type: 'thread_event', data: { ...entry('e2', ''), kind: 'task', payload: '{"event":"background_tasks_changed"}' } }));
        await waitFor(() => expect(calls('/api/v1/threads/{id}')).toBe(thread0 + 1));
        expect(calls('/api/v1/threads/{id}/events')).toBe(1);
    });
});
