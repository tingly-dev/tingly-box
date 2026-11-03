import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ProjectSettingsDialog } from './ProjectSettingsDialog';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn(), patch: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { GET: m.get, PATCH: m.patch } }));
vi.mock('../../hooks/useModels', () => ({ useModels: () => ({ models: [] }) }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const project = { id: 'p', type: 'project', name: 'P', goal: '', instructions: '', repos: [], coordinator_model: '', thread_model: '' };
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function open(p: Record<string, unknown> = project) {
    m.get.mockResolvedValue({ data: { default_max_running_threads: 4, max_running_threads_limit: 100 }, response: new Response() });
    m.patch.mockResolvedValue({ data: { ...p }, response: new Response() });
    render(<ProjectsHostProvider host={host}><ProjectSettingsDialog project={p as never} open tab="general" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></ProjectsHostProvider>);
    return screen.getByRole('combobox', { name: 'projectSettings.maxRunning' });
}

it('shows a project without the setting as the default and offers unlimited', async () => {
    const select = open();
    await waitFor(() => expect(select).toHaveTextContent('projectSettings.maxRunningDefault:4'));
    fireEvent.mouseDown(select);
    const list = within(await screen.findByRole('listbox'));
    expect(list.getByRole('option', { name: 'projectSettings.maxRunningUnlimited' })).toBeTruthy();
    expect(list.getByRole('option', { name: '2' })).toBeTruthy();
});

it('saves a chosen limit and unlimited as -1', async () => {
    const select = open();
    const save = screen.getByRole('button', { name: 'common.save' });
    expect(save).toBeDisabled();
    fireEvent.mouseDown(select);
    fireEvent.click(await screen.findByRole('option', { name: '2' }));
    fireEvent.click(save);
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: expect.objectContaining({ max_running_threads: 2 }) }));

    fireEvent.mouseDown(select);
    fireEvent.click(await screen.findByRole('option', { name: 'projectSettings.maxRunningUnlimited' }));
    fireEvent.click(save);
    await waitFor(() => expect(m.patch).toHaveBeenLastCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: expect.objectContaining({ max_running_threads: -1 }) }));
});

it('keeps a stored value that is not a preset selectable', async () => {
    const select = open({ ...project, max_running_threads: 7 });
    expect(select).toHaveTextContent('7');
});
