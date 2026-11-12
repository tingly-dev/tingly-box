import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import ProjectPage from './ProjectPage';
import { useProjectEvents } from '../hooks/useProjectEvents';
import { ProjectsHostProvider } from '../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn(), patch: vi.fn(), placement: { overview: 'side', thread: 'side', overviewFloat: false, setThread: vi.fn() } }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../client', async (actual) => ({ ...await actual<typeof import('../client')>(), api: { GET: m.get, PUT: m.put, POST: m.post, PATCH: m.patch } }));
vi.mock('../hooks/useProjectEvents', () => ({ useProjectEvents: vi.fn() }));
vi.mock('../hooks/useTranscript', () => ({ useTranscript: () => ({ events: [], apply: vi.fn(), edit: vi.fn(), catchUp: vi.fn() }) }));
vi.mock('../components/project/ConversationPane', () => ({ ConversationPane: ({ composerContext, disabledReason, onSend, busy }: { composerContext: React.ReactNode; disabledReason: React.ReactNode; onSend: (text: string, attachments: never[]) => Promise<boolean>; busy: boolean }) => <div data-testid="defaults" data-busy={busy}>{disabledReason}{composerContext}<button onClick={() => void onSend("hello", [])}>send message</button><button onClick={() => void onSend("/clear", [])}>new session</button></div> }));
vi.mock('../components/project/OverviewPane', () => ({ OverviewPane: ({ onOpen, status }: { onOpen: (id: string) => void; status?: React.ReactNode }) => <><button onClick={() => onOpen('a')}>open thread</button><div data-testid="overview-status">{status}</div></> }));
vi.mock('../components/workspace/WorkspaceLayout', () => ({ useWorkspacePane: () => ({ narrow: false, reveal: vi.fn() }), WorkspaceLayout: ({ main, side }: { main: React.ReactNode; side: React.ReactNode }) => <>{main}{side}</> }));
vi.mock('../hooks/usePlacement', () => ({ usePlacement: () => m.placement }));
vi.mock('../components/project/ThreadPanel', () => ({ ThreadPanel: ({ composerContext, modelControl, effortControl, belowComposer, settingsSaving, canSend, onBack, liveTasks }: { liveTasks?: unknown[]; composerContext: React.ReactNode; modelControl: React.ReactNode; effortControl: React.ReactNode; belowComposer: React.ReactNode; settingsSaving: boolean; canSend: () => boolean; onBack: () => void }) => <div data-testid="thread" data-live-tasks={liveTasks?.length ?? 0} data-saving={settingsSaving} data-can-send={canSend()}><button onClick={onBack}>back</button>{modelControl}{effortControl}{composerContext}{belowComposer}</div> }));
vi.mock('../components/ModelPicker', () => ({ ModelPicker: ({ value, defaultLabel, disabled, onChange }: { value: string; defaultLabel?: string; disabled?: boolean; onChange: (v: string) => void }) => <button disabled={disabled} onClick={() => onChange('worker-choice')}>{value || defaultLabel}</button> }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const p = { id: 'p', type: 'project', main_thread_id: 'main', name: 'Project', thread_model: 'project-choice', thread_permission_mode: 'auto' };
const a = { id: 'a', project_id: 'p', model: 'observed-upstream', status: 'idle' };
const b = { id: 'b', project_id: 'p', model_override: 'sibling-choice', status: 'idle' };
function Switch() { const nav = useNavigate(); return <button onClick={() => nav('/projects/p?thread=b')}>switch</button>; }
afterEach(() => { cleanup(); localStorage.clear(); vi.clearAllMocks(); });

it('keeps the coordinator idle when /clear is rejected', async () => {
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: p, threads: [], coordinator_busy: false }
        : url.endsWith('permission-modes') ? { modes: [] } : {}, response: new Response() }));
    m.post.mockResolvedValue({ error: { error: '/clear is not available for the project coordinator' }, response: new Response(null, { status: 400 }) });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: 'new session' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/messages', expect.objectContaining({ body: { content: '/clear', attachments: [] } })));
    expect(screen.getByTestId('defaults')).toHaveAttribute('data-busy', 'false');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

it('keeps the coordinator idle when an older server accepts /clear synchronously', async () => {
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: p, threads: [], coordinator_busy: false }
        : url.endsWith('permission-modes') ? { modes: [] } : {}, response: new Response() }));
    m.post.mockResolvedValue({ data: { id: 'main' }, response: new Response() });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: 'new session' }));
    await waitFor(() => expect(m.post).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByTestId('defaults')).toHaveAttribute('data-busy', 'false'));
});

it('asks before retrying an interrupted coordinator input', async () => {
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: p, threads: [], coordinator_busy: false, coordinator_recovery: 'input-1' }
        : url.endsWith('permission-modes') ? { modes: [] } : {}, response: new Response() }));
    m.post.mockResolvedValue({ data: { project: p, threads: [], coordinator_busy: false }, response: new Response() });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: 'conversation.recoveryRetry' }));
    const dialog = screen.getByRole('dialog');
    expect(within(dialog).getByText('conversation.recoveryRetryConfirm')).toBeInTheDocument();
    expect(m.post).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole('button', { name: 'conversation.recoveryRetry' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/projects/{id}/coordinator/recover', { params: { path: { id: 'p' } }, body: { input_id: 'input-1', action: 'retry' } }));
    await waitFor(() => expect(screen.queryByText('conversation.recoveryMessage')).not.toBeInTheDocument());
});

it('hands the coordinator status to the Overview as its Status tab', async () => {
    const main = { id: 'main', project_id: 'p', status: 'working', turns: 5, input_tokens: 1000, output_tokens: 200, cache_read_tokens: 500, cache_write_tokens: 0, context_tokens: 57500, context_window: 200000, last_turn_ms: 4200 };
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: p, threads: [a], coordinator: main, coordinator_busy: true, coordinator_queued: 3 }
        : url.endsWith('permission-modes') ? { modes: [] } : {}, response: new Response() }));
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    const status = await screen.findByTestId('overview-status');
    await waitFor(() => expect(within(status).getByText('57.5k / 200k · 29%')).toBeInTheDocument());
    const card = within(status).getByRole('region', { name: 'rail.status' });
    expect(within(card).getByText('thread.status.working')).toBeInTheDocument();
    expect(within(card).getByText('3')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'rail.coordinatorShow' })).not.toBeInTheDocument();
    const onEvent = vi.mocked(useProjectEvents).mock.lastCall![1];
    act(() => onEvent({ type: 'thread', project_id: 'p', data: { ...main, turns: 0, context_tokens: 0, context_window: 0 } }));
    expect(within(status).queryByText('57.5k / 200k · 29%')).not.toBeInTheDocument();
});

it('uses independent worker values, guards saves across thread switching and never patches project defaults', async () => {
    m.get.mockImplementation(async (url: string, options: { params?: { path: { id: string } } }) => ({ data:
        url === '/api/v1/projects/{id}' ? { project: p, threads: [a, b], coordinator_busy: false }
            : url.endsWith('permission-modes') ? { modes: ['auto', 'plan'] }
                : { thread: options.params?.path.id === 'a' ? a : b }, response: new Response() }));
    let finish!: (value: unknown) => void;
    m.put.mockReturnValue(new Promise((r) => { finish = r; }));
    render(<MemoryRouter initialEntries={['/projects/p?thread=a']}><ProjectsHostProvider host={host}><Switch /><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    const choose = await screen.findByRole('button', { name: 'threadSettings.projectDefault · project-choice' });
    expect(screen.queryByRole('button', { name: 'observed-upstream' })).not.toBeInTheDocument();
    fireEvent.click(choose);
    expect(m.put).toHaveBeenCalledWith('/api/v1/threads/{id}/model', { params: { path: { id: 'a' } }, body: { model: 'worker-choice' } });
    expect(screen.getByTestId('thread')).toHaveAttribute('data-saving', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'switch' }));
    await screen.findByRole('button', { name: 'sibling-choice' });
    await act(async () => finish({ data: { ...a, model_override: 'worker-choice' }, response: new Response() }));
    expect(screen.getByRole('button', { name: 'sibling-choice' })).not.toBeDisabled();
    expect(m.patch).not.toHaveBeenCalled();
    expect(screen.getByTestId('defaults')).toHaveTextContent('project-choice');
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'threadSettings.defaultPermissions' }));
    await waitFor(() => expect(screen.queryByRole('option', { name: 'session.permissionDefault' })).not.toBeInTheDocument());
});


it('switches the main surface to a thread and returns to the preserved project conversation', async () => {
    m.placement.thread = 'main';
    m.get.mockImplementation(async (url: string) => ({ data:
        url === '/api/v1/projects/{id}' ? { project: p, threads: [a], coordinator_busy: false }
            : url.endsWith('permission-modes') ? { modes: ['auto'] } : { thread: a }, response: new Response() }));
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    await screen.findByTestId('defaults');
    fireEvent.click(screen.getByRole('button', { name: 'open thread' }));
    expect(await screen.findByTestId('thread')).toBeVisible();
    expect(screen.getByTestId('defaults')).not.toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'back' }));
    expect(screen.getByTestId('defaults')).toBeVisible();
    expect(screen.queryByTestId('thread')).not.toBeInTheDocument();
    m.placement.thread = 'side';
});


it('lets standalone users recover an imported project and worker profile', async () => {
    const imported = { ...p, coordinator_profile: 'tb-coordinator', thread_profile: 'tb-workers', coordinator_model: 'opus' };
    const worker = { ...a, profile_override: 'tb-worker' };
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: imported, threads: [worker], coordinator_busy: false }
        : url.endsWith('permission-modes') ? { modes: ['auto'] } : { thread: worker }, response: new Response() }));
    m.patch.mockResolvedValue({ data: { ...imported, coordinator_profile: '', coordinator_model: '' }, response: new Response() });
    m.put.mockResolvedValue({ data: { ...worker, profile_override: '' }, response: new Response() });
    render(<MemoryRouter initialEntries={['/projects/p?thread=a']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    await screen.findByText('threadSettings.coordinatorProfileUnavailable');
    expect(screen.getByText('threadSettings.defaultProfileUnavailable')).toBeInTheDocument();
    expect(screen.getByText('threadSettings.threadProfileUnavailable')).toBeInTheDocument();
    const buttons = screen.getAllByRole('button', { name: 'threadSettings.useConnection' });
    fireEvent.click(buttons[2]);
    await waitFor(() => expect(m.put).toHaveBeenCalledWith('/api/v1/threads/{id}/profile', { params: { path: { id: 'a' } }, body: { profile: '', inherit: false } }));
    fireEvent.click(buttons[0]);
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: { coordinator_profile: '', coordinator_model: '' } }));
});

it('changes the project workspace default from the composer settings', async () => {
    let project = { ...p, disable_worktree: false };
    m.get.mockImplementation(async (url: string) => ({ data:
        url === '/api/v1/projects/{id}' ? { project, threads: [], coordinator_busy: false }
            : url.endsWith('permission-modes') ? { modes: ['auto'] } : {}, response: new Response() }));
    m.patch.mockImplementation(async (_url: string, options: { body: { disable_worktree: boolean } }) => {
        project = { ...project, disable_worktree: options.body.disable_worktree };
        return { data: project, response: new Response() };
    });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    const picker = await screen.findByRole('combobox', { name: 'threadSettings.worktreeMode' });
    expect(picker).toHaveTextContent('threadSettings.workspace.worktree');
    fireEvent.mouseDown(picker);
    fireEvent.click(screen.getByRole('option', { name: 'threadSettings.workspace.shared' }));
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: { disable_worktree: true } }));
    await waitFor(() => expect(picker).toHaveTextContent('threadSettings.workspace.shared'));
    fireEvent.mouseDown(picker);
    fireEvent.click(screen.getByRole('option', { name: 'threadSettings.workspace.worktree' }));
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: { disable_worktree: false } }));
    await waitFor(() => expect(picker).toHaveTextContent('threadSettings.workspace.worktree'));
});


it('asks once before the first message, saves the chosen mode before sending, and allows cancellation', async () => {
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: { ...p, disable_worktree: false }, threads: [], coordinator_busy: false }
        : url.endsWith('permission-modes') ? { modes: ['auto'] } : {}, response: new Response() }));
    m.patch.mockResolvedValue({ data: { ...p, disable_worktree: true }, response: new Response() });
    m.post.mockResolvedValue({ data: {}, response: new Response() });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: 'send message' }));
    expect(await screen.findByRole('radio', { name: 'worktreeNotice.worktree' })).toBeChecked();
    fireEvent.click(screen.getByRole('button', { name: 'common.cancel' }));
    expect(m.post).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole('button', { name: 'send message' }));
    fireEvent.click(await screen.findByRole('radio', { name: 'worktreeNotice.shared' }));
    fireEvent.click(screen.getByRole('button', { name: 'worktreeNotice.continue' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledTimes(1));
    expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: { disable_worktree: true } });
    expect(m.patch.mock.invocationCallOrder[0]).toBeLessThan(m.post.mock.invocationCallOrder[0]);
    fireEvent.click(await screen.findByRole('button', { name: 'send message' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
});

it('does not send or dismiss the notice permanently when saving the chosen mode fails', async () => {
    m.get.mockImplementation(async (url: string) => ({ data: url === '/api/v1/projects/{id}'
        ? { project: { ...p, disable_worktree: false }, threads: [], coordinator_busy: false }
        : url.endsWith('permission-modes') ? { modes: ['auto'] } : {}, response: new Response() }));
    m.patch.mockResolvedValue({ error: { error: 'save failed' }, response: new Response(null, { status: 500 }) });
    render(<MemoryRouter initialEntries={['/projects/p']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    fireEvent.click(await screen.findByRole('button', { name: 'send message' }));
    fireEvent.click(await screen.findByRole('radio', { name: 'worktreeNotice.shared' }));
    fireEvent.click(screen.getByRole('button', { name: 'worktreeNotice.continue' }));
    await waitFor(() => expect(m.patch).toHaveBeenCalled());
    expect(m.post).not.toHaveBeenCalled();
    expect(localStorage.getItem('tingly-project.worktreeNoticeSeen')).toBeNull();
});

it('hands a worker\'s running background tasks to its panel, and reads them again when they change', async () => {
    const running = { ...a, status: 'working', running_tasks: 2 };
    const tasks = [{ task_id: 'b1', task_type: 'local_bash', description: 'pnpm test' }, { task_id: 'a1', task_type: 'local_agent', description: 'review' }];
    m.get.mockImplementation(async (url: string) => ({ data:
        url === '/api/v1/projects/{id}' ? { project: p, threads: [running], coordinator_busy: false }
            : url.endsWith('permission-modes') ? { modes: ['auto'] } : { thread: running, running: true, background_tasks: tasks }, response: new Response() }));
    render(<MemoryRouter initialEntries={['/projects/p?thread=a']}><ProjectsHostProvider host={host}><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    await waitFor(() => expect(screen.getByTestId('thread')).toHaveAttribute('data-live-tasks', '2'));

    const threadReads = () => m.get.mock.calls.filter(([url]) => url === '/api/v1/threads/{id}').length;
    const before = threadReads();
    const onEvent = vi.mocked(useProjectEvents).mock.calls.at(-1)![1];
    // Progress is noise; a change in the set is read again.
    act(() => onEvent({ type: 'thread_event', data: { thread_id: 'a', kind: 'task', payload: JSON.stringify({ event: 'task_progress' }) } } as never));
    expect(threadReads()).toBe(before);
    act(() => onEvent({ type: 'thread_event', data: { thread_id: 'a', kind: 'task', payload: JSON.stringify({ event: 'background_tasks_changed', tasks: [] }) } } as never));
    await waitFor(() => expect(threadReads()).toBe(before + 1));
});

it('does not show the tasks of a thread the user already left', async () => {
    const tasksOf = { a: [{ task_id: 'a1', task_type: 'local_bash', description: 'a task' }], b: [] as unknown[] };
    let answerA!: () => void;
    m.get.mockImplementation(async (url: string, options: { params?: { path: { id: string } } }) => {
        if (url === '/api/v1/projects/{id}') return { data: { project: p, threads: [a, b], coordinator_busy: false }, response: new Response() };
        if (url.endsWith('permission-modes')) return { data: { modes: ['auto'] }, response: new Response() };
        const tid = options.params?.path.id as 'a' | 'b';
        if (tid === 'a') await new Promise<void>((r) => { answerA = r; });
        return { data: { thread: tid === 'a' ? a : b, running: true, background_tasks: tasksOf[tid] }, response: new Response() };
    });
    render(<MemoryRouter initialEntries={['/projects/p?thread=a']}><ProjectsHostProvider host={host}><Switch /><Routes><Route path="/projects/:id" element={<ProjectPage />} /></Routes></ProjectsHostProvider></MemoryRouter>);
    await waitFor(() => expect(answerA).toBeDefined());
    fireEvent.click(screen.getByRole('button', { name: 'switch' }));
    await waitFor(() => expect(m.get.mock.calls.filter(([u, o]) => u === '/api/v1/threads/{id}' && o.params.path.id === 'b').length).toBeGreaterThan(0));
    await act(async () => answerA());
    await waitFor(() => expect(screen.getByTestId('thread')).toHaveAttribute('data-live-tasks', '0'));
});
