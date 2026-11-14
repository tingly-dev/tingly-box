import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, type ProjectSummary, type Thread, type ThreadEvent } from '../../client';
import { ProjectsHostProvider, type ProjectsHost } from '../../host/ProjectsHost';
import { HostNotifyProvider } from '../../hooks/useNotify';
import { NEEDS_YOU_SHOWN, NeedsYou } from './NeedsYou';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, o?: { defaultValue?: string; count?: number; tool?: string; title?: string }) =>
            (o?.defaultValue ?? key).replace('{{tool}}', o?.tool ?? '').replace('{{count}}', String(o?.count ?? '')).replace('{{title}}', o?.title ?? ''),
        i18n: { language: 'en' },
    }),
}));

const m = vi.hoisted(() => ({ all: null as unknown[] | null, onEvent: undefined as ((ev: unknown) => void) | undefined }));
vi.mock('../../hooks/useProjectList', () => ({
    useProjectList: (_enabled: boolean, onEvent?: (ev: unknown) => void) => {
        m.onEvent = onEvent;
        return { all: m.all, projects: m.all, folders: [], waiting: 0, reload: vi.fn() };
    },
}));

const counts = (waiting: number, failed = 0) => ({ working: 0, waiting, failed, ready: 0, idle: 0, resolved: 0 });
const project = (id: string, name: string, waiting: number, failed = 0) =>
    ({ id, type: 'project', name, main_thread_id: `m-${id}`, threads: counts(waiting, failed), updated_at: '2026-11-13T00:00:00Z' }) as unknown as ProjectSummary;
const folder = (id: string, path: string, waiting: number) =>
    ({ id, type: 'folder', name: id, path, threads: counts(waiting), updated_at: '2026-11-13T00:00:00Z' }) as unknown as ProjectSummary;
const thread = (o: Partial<Thread>): Thread =>
    ({ id: 't1', project_id: 'p1', title: 'Fix login', status: 'waiting', wait_reason: 'Approve: Bash', running_tasks: 0, updated_at: '2026-11-13T10:00:00Z', ...o }) as Thread;
const approval = (thread_id: string, id: string, command: string): ThreadEvent => ({
    id: `e-${id}`, thread_id, kind: 'approval', content: 'Bash',
    payload: JSON.stringify({ request_id: id, tool: 'Bash', input: { command }, state: 'pending' }),
    created_at: '2026-11-13T10:00:00Z',
} as ThreadEvent);

const ok = (data: unknown) => ({ data, response: new Response('{}', { status: 200 }) });
const host: ProjectsHost = {
    paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: (p) => (p ? `/projects/folder?path=${p}` : '/projects/folder') },
};
const Where = () => {
    const l = useLocation();
    return <div data-testid="where">{l.pathname + l.search}</div>;
};

let threadsOf: Record<string, Thread[]>;
let requestsOf: Record<string, ThreadEvent[]>;
const post = vi.fn();
const show = () => render(
    <MemoryRouter initialEntries={['/workspace']}>
        <ProjectsHostProvider host={host}>
            <HostNotifyProvider notify={vi.fn()}>
                <Routes><Route path="/workspace" element={<NeedsYou />} /><Route path="*" element={<Where />} /></Routes>
            </HostNotifyProvider>
        </ProjectsHostProvider>
    </MemoryRouter>,
);

beforeEach(() => {
    post.mockReset();
    post.mockResolvedValue(ok({}));
    vi.spyOn(api, 'GET').mockImplementation((async (path: string, init?: { params?: { path?: { id?: string } } }) => {
        const id = init?.params?.path?.id ?? '';
        if (path === '/api/v1/projects/{id}/threads') return ok({ threads: threadsOf[id] ?? [] });
        if (path === '/api/v1/threads/{id}/requests') return ok({ requests: requestsOf[id] ?? [] });
        return ok({});
    }) as never);
    vi.spyOn(api, 'POST').mockImplementation(post as never);
    threadsOf = {};
    requestsOf = {};
});
afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
});

describe('NeedsYou', () => {
    it('shows nothing when no project has anything waiting', () => {
        m.all = [project('p1', 'Payments', 0)];
        show();
        expect(screen.queryByRole('region')).not.toBeInTheDocument();
        expect(api.GET).not.toHaveBeenCalled();
    });

    it('lists waiting and failed threads across projects and folders, reading only the projects that have some', async () => {
        m.all = [project('p1', 'Payments', 1, 1), folder('f1', '/w/app', 1), project('p2', 'Quiet', 0)];
        threadsOf = {
            p1: [thread({ id: 't1', title: 'Pick backoff', updated_at: '2026-11-13T09:00:00Z' }), thread({ id: 't2', title: 'Crashed run', status: 'failed', updated_at: '2026-11-13T11:00:00Z' }), thread({ id: 't3', title: 'Fine', status: 'ready' })],
            f1: [thread({ id: 's1', project_id: 'f1', title: 'Add toggle', updated_at: '2026-11-13T10:00:00Z' })],
        };
        show();
        const block = within(await screen.findByRole('region', { name: 'needsYou.title' }));
        expect(await block.findByText('Add toggle')).toBeInTheDocument();
        // Waiting first (newest of those first), then the failed one; threads that need nothing are left out.
        const titles = block.getAllByText(/Add toggle|Pick backoff|Crashed run|Fine/).map((e) => e.textContent);
        expect(titles).toEqual(['Add toggle', 'Pick backoff', 'Crashed run']);
        expect(block.getByText('needsYou.failed')).toBeInTheDocument();
        expect(vi.mocked(api.GET).mock.calls.filter((c) => c[0] === '/api/v1/projects/{id}/threads').map((c) => (c[1] as { params: { path: { id: string } } }).params.path.id).sort()).toEqual(['f1', 'p1']);
    });

    it('answers an approval in place with the thread page card', async () => {
        m.all = [project('p1', 'Payments', 1)];
        threadsOf = { p1: [thread({ id: 't1' })] };
        requestsOf = { t1: [approval('t1', 'r1', 'go test ./...')] };
        show();
        fireEvent.click(await screen.findByRole('button', { name: 'Allow' }));
        await waitFor(() => expect(post).toHaveBeenCalled());
        expect(post.mock.calls[0][0]).toBe('/api/v1/threads/{id}/respond');
        expect(post.mock.calls[0][1]).toMatchObject({ params: { path: { id: 't1' } }, body: { request_id: 'r1', allow: true } });
    });

    it('opens the first row with requests and folds it on a click; the others stay one line', async () => {
        m.all = [project('p1', 'Payments', 2)];
        threadsOf = { p1: [thread({ id: 't1', title: 'One', updated_at: '2026-11-13T11:00:00Z' }), thread({ id: 't2', title: 'Two', updated_at: '2026-11-13T10:00:00Z' })] };
        requestsOf = { t1: [approval('t1', 'r1', 'ls')], t2: [approval('t2', 'r2', 'pwd')] };
        show();
        const one = await screen.findByRole('button', { name: 'One', expanded: true });
        expect(screen.getByRole('button', { name: 'Two', expanded: false })).toBeInTheDocument();
        expect(screen.getAllByRole('button', { name: 'Allow' })).toHaveLength(1);
        fireEvent.click(one);
        expect(screen.queryByRole('button', { name: 'Allow' })).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Two' }));
        expect(screen.getAllByRole('button', { name: 'Allow' })).toHaveLength(1);
    });

    it('opens a thread where a notification would: a project thread, or a folder session', async () => {
        m.all = [project('p1', 'Payments', 1), folder('f1', '/w/app', 1)];
        threadsOf = { p1: [thread({ id: 't1', title: 'Proj thread', updated_at: '2026-11-13T11:00:00Z' })], f1: [thread({ id: 's1', project_id: 'f1', title: 'Folder one', updated_at: '2026-11-13T10:00:00Z' })] };
        show();
        const rows = await screen.findAllByRole('button', { name: 'needsYou.open' });
        fireEvent.click(rows[0]);
        expect(screen.getByTestId('where')).toHaveTextContent('/projects/p1?thread=t1');
        cleanup();
        show();
        fireEvent.click((await screen.findAllByRole('button', { name: 'needsYou.open' }))[1]);
        expect(screen.getByTestId('where')).toHaveTextContent('/projects/folder?path=/w/app&session=s1');
    });

    it('caps the list and expands it on request', async () => {
        m.all = [project('p1', 'Payments', NEEDS_YOU_SHOWN + 2)];
        threadsOf = { p1: Array.from({ length: NEEDS_YOU_SHOWN + 2 }, (_, i) => thread({ id: `t${i}`, title: `Thread ${i}`, updated_at: `2026-11-13T1${i}:00:00Z` })) };
        show();
        await screen.findByText('Thread 4');
        expect(screen.getAllByRole('button', { name: 'needsYou.open' })).toHaveLength(NEEDS_YOU_SHOWN);
        fireEvent.click(screen.getByRole('button', { name: 'needsYou.more' }));
        expect(screen.getAllByRole('button', { name: 'needsYou.open' })).toHaveLength(NEEDS_YOU_SHOWN + 2);
        fireEvent.click(screen.getByRole('button', { name: 'needsYou.fewer' }));
        expect(screen.getAllByRole('button', { name: 'needsYou.open' })).toHaveLength(NEEDS_YOU_SHOWN);
    });
});
