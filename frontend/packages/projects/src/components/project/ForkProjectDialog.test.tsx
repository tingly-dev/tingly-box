import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { ForkProjectDialog } from './ForkProjectDialog';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const m = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { name?: string }) => (o?.name != null ? `${key}:${o.name}` : key) }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { POST: m.post } }));
const host = { paths: { list: '/projects', project: (id: string) => `/projects/${id}`, folder: () => '/folders' } };
const project = { id: 'p', type: 'project', name: 'Origin', goal: '', instructions: '', repos: [], coordinator_model: '', thread_model: '', max_running_threads: 4 };
const fork = { id: 'f', name: 'Origin (fork)' };
afterEach(() => { cleanup(); vi.clearAllMocks(); });

// Where shows the route the page is on now, so the move to the fork is visible.
function Where() {
    return <div data-testid="where">{useLocation().pathname}</div>;
}

function open() {
    render(
        <ProjectsHostProvider host={host}>
            <MemoryRouter initialEntries={['/projects/p']}>
                <Routes>
                    <Route path="/projects/p" element={<ForkProjectDialog project={project as never} open onClose={vi.fn()} />} />
                    <Route path="/projects/f" element={<div>the fork</div>} />
                    <Route path="*" element={<Where />} />
                </Routes>
            </MemoryRouter>
        </ProjectsHostProvider>,
    );
    return screen.getByRole('textbox', { name: 'project.name' }) as HTMLInputElement;
}

it('prefers "<source> (fork)" as the default name and explains what is copied', () => {
    const field = open();
    expect(field.value).toBe('Origin (fork)');
    expect(screen.getByText('projectSettings.forkCopies')).toBeTruthy();
    expect(screen.getByText('projectSettings.forkNotCopied')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'projectSettings.fork' })).toBeEnabled();
});

it('forks under the given name and moves to the new project', async () => {
    m.post.mockResolvedValue({ data: fork, response: new Response() });
    const field = open();
    fireEvent.change(field, { target: { value: '  Round two  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'projectSettings.fork' }));
    await waitFor(() => expect(m.post).toHaveBeenCalledWith('/api/v1/projects/{id}/fork', { params: { path: { id: 'p' } }, body: { name: 'Round two' } }));
    await waitFor(() => expect(screen.getByText('the fork')).toBeTruthy());
});
