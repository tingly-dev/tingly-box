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
    localStorage.clear();
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
    it('lists projects starred first, marks the open one and dots what waits', async () => {
        show(host([summary('a', 'Alpha', 0), summary('b', 'Beta', 2, true), ...folders]), '/p/a');
        const projects = await screen.findByRole('region', { name: 'nav.projects' });
        await waitFor(() => expect(within(projects).getAllByRole('link').map((l) => l.textContent)).toEqual(['nav.projects', 'Beta', 'Alpha']));
        expect(within(projects).getByRole('link', { name: 'Alpha' })).toHaveAttribute('aria-current', 'page');
        expect(within(projects).getByRole('link', { name: /Beta/ })).toHaveAttribute('href', '/p/b');
        // What waits in a project dots its row; nothing waiting, no dot.
        expect(within(projects).getByRole('link', { name: /Beta/ }).querySelector('.MuiBadge-badge')).not.toHaveClass('MuiBadge-invisible');
        expect(within(projects).getByRole('link', { name: 'Alpha' }).querySelector('.MuiBadge-badge')).toHaveClass('MuiBadge-invisible');
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

    it('keeps projects flat and expands folders to show sessions', async () => {
        const h = host([summary('a', 'Alpha', 0), folder('f-app', '/home/me/app', 1, 0), folder('f-notes', '/home/me/notes', 0, 1)]);
        list.mockImplementation((path: string) => Promise.resolve({
            data: path.endsWith('/threads')
                ? { threads: [
                    { id: 't-run', title: 'Running one', status: 'working', updated_at: '2026-10-22T00:00:00Z' },
                    { id: 't-wait', title: 'Waiting one', status: 'waiting', updated_at: '2026-10-21T00:00:00Z' },
                ] }
                : { projects: [summary('a', 'Alpha', 0), folder('f-app', '/home/me/app', 1, 0), folder('f-notes', '/home/me/notes', 0, 1)] },
            response: new Response(),
        }));
        show(h, '/f?path=%2Fhome%2Fme%2Fapp&session=t-run', { showWorkdirs: true });
        const region = await screen.findByRole('region', { name: 'workdir.title' });
        const session = await within(region).findByRole('link', { name: /Running one/ });
        expect(session).toHaveAttribute('href', '/f?path=%2Fhome%2Fme%2Fapp&session=t-run');
        expect(session).toHaveAttribute('aria-current', 'page');
        expect(within(region).getAllByRole('link').map((link) => link.textContent)).toEqual(expect.arrayContaining([expect.stringContaining('Waiting one'), expect.stringContaining('Running one')]));
        expect(within(region).getByText('Waiting one').closest('a')!.compareDocumentPosition(session) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
        fireEvent.click(within(region).getByRole('button', { name: 'sidebar.fold' }));
        expect(localStorage.getItem('tingly-project.nav.folder.f-app')).toBe('folded');
        expect(within(region).queryByText('Running one')).not.toBeInTheDocument();
        fireEvent.click(within(region).getAllByRole('button', { name: 'sidebar.unfold' })[0]);
        expect(await within(region).findByText('Running one')).toBeInTheDocument();
        const projects = screen.getByRole('region', { name: 'nav.projects' });
        expect(within(projects).queryByRole('button', { name: /sidebar\.(fold|unfold)/ })).not.toBeInTheDocument();
    });
    it('limits older sessions while keeping urgent and selected sessions visible', async () => {
        const h = host([folder('f-app', '/home/me/app', 1)]);
        const threads = Array.from({ length: 7 }, (_, i) => ({ id: `t-${i}`, title: `Session ${i}`, status: i === 6 ? 'waiting' : 'idle', updated_at: `2026-10-${String(20 - i).padStart(2, '0')}T00:00:00Z` }));
        list.mockImplementation((path: string) => Promise.resolve({ data: path.endsWith('/threads') ? { threads } : { projects: [folder('f-app', '/home/me/app', 1)] }, response: new Response() }));
        show(h, '/f?path=%2Fhome%2Fme%2Fapp&session=t-5', { showWorkdirs: true });
        expect(await screen.findByText('Session 6')).toBeInTheDocument();
        expect(screen.getByText('Session 5')).toBeInTheDocument();
        const older = screen.getByRole('button', { name: /sidebar.showOlder/ });
        fireEvent.click(older);
        expect(screen.getByText('Session 4')).toBeInTheDocument();
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
