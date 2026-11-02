import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useNavigate } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import ProjectPage from './ProjectPage';
import { ProjectsHostProvider } from '../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), patch: vi.fn(), placement: { overview: 'side', thread: 'side', overviewFloat: false, setThread: vi.fn() } }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../client', async (actual) => ({ ...await actual<typeof import('../client')>(), api: { GET: m.get, PUT: m.put, PATCH: m.patch } }));
vi.mock('../hooks/useProjectEvents', () => ({ useProjectEvents: vi.fn() }));
vi.mock('../hooks/useTranscript', () => ({ useTranscript: () => ({ events: [], apply: vi.fn(), edit: vi.fn(), catchUp: vi.fn() }) }));
vi.mock('../components/project/ConversationPane', () => ({ ConversationPane: ({ composerContext, disabledReason }: { composerContext: React.ReactNode; disabledReason: React.ReactNode }) => <div data-testid="defaults">{disabledReason}{composerContext}</div> }));
vi.mock('../components/project/OverviewPane', () => ({ OverviewPane: ({ onOpen }: { onOpen: (id: string) => void }) => <button onClick={() => onOpen('a')}>open thread</button> }));
vi.mock('../components/workspace/WorkspaceLayout', () => ({ useWorkspacePane: () => ({ narrow: false, reveal: vi.fn() }), WorkspaceLayout: ({ main, side }: { main: React.ReactNode; side: React.ReactNode }) => <>{main}{side}</> }));
vi.mock('../hooks/usePlacement', () => ({ usePlacement: () => m.placement }));
vi.mock('../components/project/ThreadPanel', () => ({ ThreadPanel: ({ composerContext, modelControl, effortControl, belowComposer, settingsSaving, canSend, onBack }: { composerContext: React.ReactNode; modelControl: React.ReactNode; effortControl: React.ReactNode; belowComposer: React.ReactNode; settingsSaving: boolean; canSend: () => boolean; onBack: () => void }) => <div data-testid="thread" data-saving={settingsSaving} data-can-send={canSend()}><button onClick={onBack}>back</button>{modelControl}{effortControl}{composerContext}{belowComposer}</div> }));
vi.mock('../components/ModelPicker', () => ({ ModelPicker: ({ value, defaultLabel, disabled, onChange }: { value: string; defaultLabel?: string; disabled?: boolean; onChange: (v: string) => void }) => <button disabled={disabled} onClick={() => onChange('worker-choice')}>{value || defaultLabel}</button> }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const p = { id: 'p', type: 'project', main_thread_id: 'main', name: 'Project', thread_model: 'project-choice', thread_permission_mode: 'auto' };
const a = { id: 'a', project_id: 'p', model: 'observed-upstream', status: 'idle' };
const b = { id: 'b', project_id: 'p', model_override: 'sibling-choice', status: 'idle' };
function Switch() { const nav = useNavigate(); return <button onClick={() => nav('/projects/p?thread=b')}>switch</button>; }
afterEach(() => { cleanup(); vi.clearAllMocks(); });

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
