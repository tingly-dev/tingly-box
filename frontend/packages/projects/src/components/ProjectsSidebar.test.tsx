import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import { ProjectsSidebar, type ProjectsSidebarSection } from './ProjectsSidebar';

const { list } = vi.hoisted(() => ({ list: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
vi.mock('../client', async (actual) => ({
    ...(await actual<typeof import('../client')>()),
    api: { GET: list },
}));
afterEach(() => {
    cleanup();
    list.mockReset();
});

const summary = (id: string, name: string, waiting: number, starred = false) => ({
    id, name, starred, type: 'project', threads: { waiting, ready: 0, working: 0, failed: 0 },
});

// A folder is a project of type folder, in the same list.
const folder = (id: string, path: string, waiting: number, working = 0) => ({
    id, name: path.split('/').pop(), path, starred: false, type: 'folder', threads: { waiting, ready: 0, working, failed: 0 },
});

const folders = [folder('f-app', '/home/me/app', 2), folder('f-notes', '/home/me/notes', 0, 1)];

const host = (projects: unknown[]): ProjectsHost => {
    list.mockImplementation(() => Promise.resolve({ data: { projects }, response: new Response() }));
    return { paths: { list: '/p', project: (id) => `/p/${id}`, folder: (path) => (path ? `/f?path=${encodeURIComponent(path)}` : '/f') } };
};

const show = (h: ProjectsHost, path: string, props: Parameters<typeof ProjectsSidebar>[0] = {}) => render(
    <MemoryRouter initialEntries={[path]}>
        <ProjectsHostProvider host={h}><ProjectsSidebar {...props} /></ProjectsHostProvider>
    </MemoryRouter>,
);

describe('ProjectsSidebar', () => {
    it('lists projects starred first, marks the open one and counts what waits', async () => {
        show(host([summary('a', 'Alpha', 0), summary('b', 'Beta', 2, true), ...folders]), '/p/a');
        const projects = await screen.findByRole('region', { name: 'nav.projects' });
        await waitFor(() => expect(within(projects).getAllByRole('link').map((l) => l.textContent)).toEqual(['nav.projects', 'Beta', 'Alpha']));
        expect(within(projects).getByRole('link', { name: 'Alpha' })).toHaveAttribute('aria-current', 'page');
        expect(within(projects).getByRole('link', { name: /Beta/ })).toHaveAttribute('href', '/p/b');
        // Beta is folded and says what waits in it with one dot.
        expect(within(projects).getByTestId('rollup')).toHaveAttribute('data-status', 'waiting');
    });

    it('renders host sections after projects and can leave projects out', async () => {
        const h = host([summary('a', 'Alpha', 0)]);
        const folders: ProjectsSidebarSection = {
            key: 'folders', title: 'Working folders', to: '/folders',
            rows: [{ key: '/src/app', label: 'app', title: '/src/app', to: '/folders?folder=app', selected: true }],
        };
        show(h, '/folders', { showProjects: false, sections: [folders] });
        const section = screen.getByRole('region', { name: 'Working folders' });
        expect(within(section).getByRole('link', { name: 'app' })).toHaveAttribute('aria-current', 'page');
        expect(screen.queryByRole('region', { name: 'nav.projects' })).not.toBeInTheDocument();
        expect(list).not.toHaveBeenCalled();
    });

    it('lists working folders as a tree: the open one unfolded, the others folded with a rollup dot', async () => {
        const h = host([folder('f-app', '/home/me/app', 1, 0), folder('f-notes', '/home/me/notes', 0, 1)]);
        list.mockImplementation((path: string) => Promise.resolve({
            data: path.endsWith('/threads')
                ? { threads: [
                    { id: 't-old', title: 'Old idle', status: 'idle', updated_at: '2026-10-01T00:00:00Z' },
                    { id: 't-done', title: 'Archived', status: 'resolved', updated_at: '2026-10-23T00:00:00Z' },
                    { id: 't-run', title: 'Running one', status: 'working', updated_at: '2026-10-22T00:00:00Z' },
                    { id: 't-wait', title: 'Waiting one', status: 'waiting', updated_at: '2026-10-21T00:00:00Z' },
                ] }
                : { projects: [folder('f-app', '/home/me/app', 1, 0), folder('f-notes', '/home/me/notes', 0, 1)] },
            response: new Response(),
        }));
        show(h, '/f?path=%2Fhome%2Fme%2Fapp&session=t-run', { showProjects: false, showWorkdirs: true });
        const region = await screen.findByRole('region', { name: 'workdir.title' });
        await waitFor(() => expect(within(region).getAllByRole('link').map((l) => l.textContent?.replace(/ago$|\d.*$/, '').trim())).toEqual(['workdir.title', 'app', 'Waiting one', 'Running one', 'Old idle', 'notes']));
        expect(within(region).getByRole('link', { name: /Running one/ })).toHaveAttribute('aria-current', 'page');
        expect(within(region).getByRole('link', { name: /Running one/ })).toHaveAttribute('href', '/f?path=%2Fhome%2Fme%2Fapp&session=t-run');
        // The archived session sits behind "Show older"; the folded folder shows what runs in it.
        expect(within(region).getByText('sidebar.showOlder')).toBeInTheDocument();
        expect(within(region).getByTestId('rollup')).toHaveAttribute('data-status', 'working');
    });
    it('remembers a folded folder per browser and unfolds on click', async () => {
        window.localStorage.clear();
        const h = host([folder('f-app', '/home/me/app', 0, 0), folder('f-notes', '/home/me/notes', 0, 0)]);
        show(h, '/f?path=%2Fhome%2Fme%2Fapp', { showProjects: false, showWorkdirs: true });
        const region = await screen.findByRole('region', { name: 'workdir.title' });
        const notes = await within(region).findByRole('button', { name: 'sidebar.unfold' });
        expect(notes).toHaveAttribute('aria-expanded', 'false');
        fireEvent.click(notes);
        expect(window.localStorage.getItem('tingly-project.nav.folder.f-notes')).toBe('open');
    });
    it('pins an IM row with the bot count when the host has IM, and leaves it out otherwise', async () => {
        const h = host([summary('a', 'Alpha', 0)]);
        const withIm: ProjectsHost = {
            ...h,
            paths: { ...h.paths, im: '/im' },
            im: { listBots: async () => [{ uuid: 'b1', name: 'A', platform: 'telegram', online: true }, { uuid: 'b2', name: 'B', platform: 'feishu', online: false }], listChats: async () => [] },
        };
        show(withIm, '/im');
        const row = await screen.findByRole('link', { name: /im\.title/ });
        expect(row).toHaveAttribute('href', '/im');
        expect(row).toHaveAttribute('aria-current', 'page');
        await waitFor(() => expect(row.textContent).toContain('im.bots'));
        cleanup();
        show(h, '/p');
        await screen.findByRole('region', { name: 'nav.projects' });
        expect(screen.queryByRole('link', { name: /im\.title/ })).not.toBeInTheDocument();
    });
});
