import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import { ProjectsSidebar, type ProjectsSidebarSection } from './ProjectsSidebar';

const { list } = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../client', async (actual) => ({
    ...(await actual<typeof import('../client')>()),
    api: { GET: list },
}));
afterEach(() => {
    cleanup();
    list.mockReset();
});

const summary = (id: string, name: string, waiting: number, starred = false) => ({
    id, name, starred, threads: { waiting, ready: 0, working: 0 },
});

const host = (projects: unknown[]): ProjectsHost => {
    list.mockResolvedValue({ data: { projects }, response: new Response() });
    return { paths: { list: '/p', project: (id) => `/p/${id}` } };
};

const show = (h: ProjectsHost, path: string, props: Parameters<typeof ProjectsSidebar>[0] = {}) => render(
    <MemoryRouter initialEntries={[path]}>
        <ProjectsHostProvider host={h}><ProjectsSidebar {...props} /></ProjectsHostProvider>
    </MemoryRouter>,
);

describe('ProjectsSidebar', () => {
    it('lists projects starred first, marks the open one and counts what waits', async () => {
        show(host([summary('a', 'Alpha', 0), summary('b', 'Beta', 2, true)]), '/p/a');
        const projects = await screen.findByRole('region', { name: 'nav.projects' });
        await waitFor(() => expect(within(projects).getAllByRole('link').map((l) => l.textContent)).toEqual(['nav.projects', 'Beta2', 'Alpha']));
        expect(within(projects).getByRole('link', { name: 'Alpha' })).toHaveAttribute('aria-current', 'page');
        expect(within(projects).getByRole('link', { name: /Beta/ })).toHaveAttribute('href', '/p/b');
    });

    it('renders host sections after projects and can leave projects out', async () => {
        const h = host([summary('a', 'Alpha', 0)]);
        const folders: ProjectsSidebarSection = {
            key: 'folders', title: 'Working folders', to: '/desk',
            rows: [{ key: '/src/app', label: 'app', title: '/src/app', to: '/desk?folder=app', selected: true }],
        };
        show(h, '/desk', { showProjects: false, sections: [folders] });
        const section = screen.getByRole('region', { name: 'Working folders' });
        expect(within(section).getByRole('link', { name: 'app' })).toHaveAttribute('aria-current', 'page');
        expect(screen.queryByRole('region', { name: 'nav.projects' })).not.toBeInTheDocument();
        expect(list).not.toHaveBeenCalled();
    });
});
