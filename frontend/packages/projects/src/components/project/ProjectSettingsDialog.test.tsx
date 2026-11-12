import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ProjectSettingsDialog } from './ProjectSettingsDialog';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn(), patch: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { max?: number }) => (o?.max != null ? `${key}:${o.max}` : key), i18n: { language: 'en' } }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { GET: m.get, PATCH: m.patch } }));
vi.mock('../../hooks/useModels', () => ({ useModels: () => ({ models: [] }) }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const project = { id: 'p', type: 'project', name: 'P', goal: '', instructions: '', repos: [], coordinator_model: '', thread_model: '', max_running_threads: 4 };
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function open(p: Record<string, unknown> = project) {
    m.get.mockResolvedValue({ data: { max_running_threads_limit: 100 }, response: new Response() });
    m.patch.mockResolvedValue({ data: { ...p }, response: new Response() });
    // The fork dialog (nested under General) navigates, so the tree needs a router.
    render(<ProjectsHostProvider host={host}><MemoryRouter><ProjectSettingsDialog project={p as never} open tab="general" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></MemoryRouter></ProjectsHostProvider>);
    return screen.getByRole('textbox', { name: 'projectSettings.maxRunning' }) as HTMLInputElement;
}
const save = () => screen.getByRole('button', { name: 'common.save' });

it('shows the stored limit as a plain number, 4 for a new project', () => {
    const field = open();
    expect(field.value).toBe('4');
    expect(screen.getByText('projectSettings.maxRunningHelp')).toBeTruthy();
    expect(save()).toBeDisabled();
});

it('saves a chosen limit', async () => {
    const field = open();
    fireEvent.change(field, { target: { value: '2' } });
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: expect.objectContaining({ max_running_threads: 2 }) }));
});

it('refuses anything but a whole number from 1 to the server limit; there is no unlimited', async () => {
    const field = open({ ...project, max_running_threads: 3 });
    expect(field.value).toBe('3');
    await waitFor(() => expect(m.get).toHaveBeenCalled());
    for (const bad of ['0', '-1', '1.5', 'many', '', '101']) {
        fireEvent.change(field, { target: { value: bad } });
        expect(save(), bad).toBeDisabled();
        expect(screen.getByText('projectSettings.maxRunningInvalid:100'), bad).toBeTruthy();
    }
    fireEvent.change(field, { target: { value: '100' } });
    expect(save()).not.toBeDisabled();
});

const usageOf = (by_model: unknown[], by_thread: unknown[]) =>
    m.get.mockImplementation((path: string) => Promise.resolve(
        path.endsWith('/usage')
            ? { data: { by_model, by_thread }, response: new Response() }
            : { data: { max_running_threads_limit: 100 }, response: new Response() },
    ));

const usageThread = { thread_id: 't1', title: 'Fix login', model: 'cc[1m]', turns: 5, input_tokens: 34_500_000, output_tokens: 98_500 };

function openUsage() {
    render(<ProjectsHostProvider host={host}><MemoryRouter><ProjectSettingsDialog project={project as never} open tab="usage" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></MemoryRouter></ProjectsHostProvider>);
}

it('renders usage as a stat strip over ranked tables with shares', async () => {
    usageOf(
        [{ model: 'cc[1m]', threads: 18, input_tokens: 305_000_000, output_tokens: 1_200_000 }],
        [usageThread, { thread_id: 't2', title: 'Split the i18n files', model: 'cc[1m]', turns: 6, input_tokens: 5_900_000, output_tokens: 50_200 }],
    );
    openUsage();
    expect(await screen.findByText('projectSettings.usage.totalTokens')).toBeTruthy();
    // The strip: compact totals with the exact figure on hover.
    expect(screen.getByTitle('40548700')).toBeTruthy();
    // The thread rows rank by tokens spent, each with its share by the meter.
    const titles = screen.getAllByText(/Fix login|Split the i18n files/).map((el) => el.textContent);
    expect(titles.indexOf('Fix login')).toBeLessThan(titles.indexOf('Split the i18n files'));
    expect(screen.getByText('85.3%')).toBeTruthy();
    expect(screen.getByText('14.7%')).toBeTruthy();
    expect(screen.getByText('305M')).toBeTruthy();
});

it('reads a project without usage as empty, not as empty tables', async () => {
    usageOf([], []);
    openUsage();
    expect(await screen.findByText('projectSettings.usage.emptyTitle')).toBeTruthy();
    expect(screen.queryByText('projectSettings.usage.totalTokens')).not.toBeInTheDocument();
});

it('saves a newly selected primary without changing other modes', async () => {
    const p = { ...project, primary_repo_path: '/git', repos: [
        { path: '/git', base_branch: 'main', kind: 'git', workspace_mode: 'worktree' },
        { path: '/docs', base_branch: '', kind: 'directory', workspace_mode: 'shared' },
    ] };
    m.get.mockResolvedValue({ data: {}, response: new Response() });
    m.patch.mockResolvedValue({ data: p, response: new Response() });
    render(<ProjectsHostProvider host={host}><MemoryRouter><ProjectSettingsDialog project={p as never} open tab="environment" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></MemoryRouter></ProjectsHostProvider>);
    expect(screen.getByRole('combobox', { name: 'repo.mode /docs' })).toHaveAttribute('aria-disabled', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'repo.makePrimary' }));
    fireEvent.click(screen.getByRole('button', { name: 'common.save' }));
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: { directories: [{ path: '/git', workspace_mode: 'worktree' }, { path: '/docs', workspace_mode: 'shared' }], primary_repo_path: '/docs' } }));
});
