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

function open(p: Record<string, unknown>) {
    const sandboxes = [
        { name: 'host', available: true, levels: {} },
        { name: 'bwrap', available: true, levels: { storage: 'confined' } },
    ];
    m.get.mockResolvedValue({ data: { max_running_threads_limit: 100, sandboxes }, response: new Response() });
    m.patch.mockResolvedValue({ data: { ...p }, response: new Response() });
    render(<ProjectsHostProvider host={host}><MemoryRouter><ProjectSettingsDialog project={p as never} open tab="runtime" onTab={vi.fn()} onClose={vi.fn()} onChanged={vi.fn()} memoryKey={0} /></MemoryRouter></ProjectsHostProvider>);
}
const save = () => screen.getByRole('button', { name: 'common.save' });
const input = (name: string) => screen.getByRole('textbox', { name: `projectSettings.quota.${name}` }) as HTMLInputElement;

it('leaves the limits empty and off on the host, with nothing suggested', async () => {
    open(project);
    await screen.findByText('projectSettings.quota.hostHelp');
    for (const f of ['memory', 'processes', 'cpu']) {
        expect(input(f).value).toBe('');
        expect(input(f)).toBeDisabled();
    }
});

it('saves limits in the server units and checks the same minimums', async () => {
    open({ ...project, thread_sandbox: 'bwrap' });
    await screen.findByText('projectSettings.quota.help');
    fireEvent.change(input('memory'), { target: { value: '256' } });
    expect(screen.getByText('projectSettings.quota.memoryMin')).toBeTruthy();
    expect(save()).toBeDisabled();
    fireEvent.change(input('memory'), { target: { value: '1024' } });
    fireEvent.change(input('processes'), { target: { value: '63' } });
    expect(screen.getByText('projectSettings.quota.processesMin')).toBeTruthy();
    fireEvent.change(input('processes'), { target: { value: '200' } });
    fireEvent.change(input('cpu'), { target: { value: '0.05' } });
    expect(screen.getByText('projectSettings.quota.cpuMin')).toBeTruthy();
    fireEvent.change(input('cpu'), { target: { value: '1.5' } });
    expect(save()).not.toBeDisabled();
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', {
        params: { path: { id: 'p' } },
        body: expect.objectContaining({ thread_memory_bytes: 1024 * 1024 ** 2, thread_tasks: 200, thread_cpu: 1500 }),
    }));
});

it('shows stored limits and clears one by emptying it', async () => {
    open({ ...project, thread_sandbox: 'bwrap', thread_memory_bytes: 2 * 1024 ** 3, thread_tasks: 128, thread_cpu: 500 });
    await screen.findByText('projectSettings.quota.help');
    expect(input('memory').value).toBe('2');
    expect(input('cpu').value).toBe('0.5');
    fireEvent.change(input('processes'), { target: { value: '' } });
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', expect.objectContaining({
        body: expect.objectContaining({ thread_memory_bytes: 2 * 1024 ** 3, thread_tasks: 0, thread_cpu: 500 }),
    })));
});

it('offers a way to switch to a sandbox in place when limits are disabled', async () => {
    open(project);
    await screen.findByText('projectSettings.quota.hostHelp');
    expect(screen.getByRole('button', { name: 'projectSettings.switchToSandbox' })).toBeTruthy();
});

it('saves the runtime tab alone, without the Basic tab fields', async () => {
    open({ ...project, thread_sandbox: 'bwrap' });
    await screen.findByText('projectSettings.quota.help');
    fireEvent.change(input('processes'), { target: { value: '200' } });
    fireEvent.click(save());
    await waitFor(() => expect(m.patch).toHaveBeenCalledWith('/api/v1/projects/{id}', expect.objectContaining({
        body: expect.objectContaining({ thread_tasks: 200 }),
    })));
    expect(m.patch.mock.calls[0][1].body).not.toHaveProperty('name');
    expect(m.patch.mock.calls[0][1].body).not.toHaveProperty('github_auto');
});
