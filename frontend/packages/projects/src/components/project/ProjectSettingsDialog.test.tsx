import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { ProjectSettingsDialog } from './ProjectSettingsDialog';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ get: vi.fn(), patch: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { max?: number }) => (o?.max != null ? `${key}:${o.max}` : key), i18n: { language: 'en' } }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { GET: m.get, PATCH: m.patch } }));
vi.mock('../../hooks/useModels', () => ({ useModels: () => ({ models: [] }) }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const project = { id: 'p', type: 'project', name: 'P', goal: '', instructions: '', repos: [], coordinator_model: '', thread_model: '' };
afterEach(() => { cleanup(); vi.clearAllMocks(); });

function open(p: Record<string, unknown> = project) {
    m.get.mockResolvedValue({ data: { max_running_threads_limit: 100 }, response: new Response() });
    m.patch.mockResolvedValue({ data: { ...p }, response: new Response() });
    render(<ProjectsHostProvider host={host}><ProjectSettingsDialog project={p as never} open tab="general" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></ProjectsHostProvider>);
    return screen.getByRole('textbox', { name: 'projectSettings.maxRunning' }) as HTMLInputElement;
}
const save = () => screen.getByRole('button', { name: 'common.save' });

it('shows a project without the setting as a plain 0 and explains that 0 is no limit', () => {
    const field = open();
    expect(field.value).toBe('0');
    expect(screen.getByText('projectSettings.maxRunningHelp')).toBeTruthy();
    expect(save()).toBeDisabled();
});

it('saves a chosen limit', async () => {
    const field = open();
    fireEvent.change(field, { target: { value: '2' } });
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: expect.objectContaining({ max_running_threads: 2 }) }));
});

it('sends 0 to remove a limit', async () => {
    const field = open({ ...project, max_running_threads: 3 });
    fireEvent.change(field, { target: { value: '0' } });
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', { params: { path: { id: 'p' } }, body: expect.objectContaining({ max_running_threads: 0 }) }));
});

it('refuses anything but a whole number from 0 to the server limit', async () => {
    const field = open({ ...project, max_running_threads: 3 });
    expect(field.value).toBe('3');
    await waitFor(() => expect(m.get).toHaveBeenCalled());
    for (const bad of ['-1', '1.5', 'many', '', '101']) {
        fireEvent.change(field, { target: { value: bad } });
        expect(save(), bad).toBeDisabled();
        expect(screen.getByText('projectSettings.maxRunningInvalid:100'), bad).toBeTruthy();
    }
    fireEvent.change(field, { target: { value: '100' } });
    expect(save()).not.toBeDisabled();
});
