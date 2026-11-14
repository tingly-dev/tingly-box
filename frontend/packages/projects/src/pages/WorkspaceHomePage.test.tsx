import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import { armWorkspaceImHint } from '../utils/imHint';
import WorkspaceHomePage, { RECENT } from './WorkspaceHomePage';

const m = vi.hoisted(() => ({ projects: null as unknown[] | null, folders: null as unknown[] | null, all: null as unknown[] | null, bindings: null as unknown[] | null }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key), i18n: { language: 'en' } }),
}));
vi.mock('../hooks/useProjectList', () => ({
    useProjectList: () => ({ projects: m.projects, folders: m.folders, all: m.all, waiting: 0, reload: vi.fn() }),
}));
vi.mock('../client/channelBindings', () => ({
    listChannelBindings: async () => m.bindings,
    bindChannel: vi.fn(),
    unbindChannel: vi.fn(),
}));

const threads = { working: 0, waiting: 0, failed: 0, ready: 0, idle: 0, resolved: 0 };
const project = (n: number) => ({
    id: `p${n}`, name: `Project ${n}`, type: 'project', goal: '', starred: false, paused: false, repos: [], threads, main_thread_id: `m${n}`,
    updated_at: `2026-11-0${n}T00:00:00Z`,
});
const folder = (n: number) => ({
    id: `f${n}`, name: `app${n}`, path: `/home/me/app${n}`, type: 'folder', starred: false, paused: false, repos: [], threads,
    updated_at: `2026-11-0${n}T00:00:00Z`,
});
const binding = (n: number, over: Record<string, unknown> = {}) => ({
    id: `b${n}`, platform: 'telegram', chat_id: `c${n}`, chat_title: `Chat ${n}`,
    project_id: 'p1', thread_id: 'm1', created_at: `2026-11-0${n}T00:00:00Z`, updated_at: `2026-11-0${n}T00:00:00Z`,
    verbose: false, ...over,
});
const host: ProjectsHost = {
    paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: (p) => (p ? `/projects/folder?path=${p}` : '/projects/folder') },
};
const hostWithIm: ProjectsHost = {
    ...host,
    paths: { ...host.paths, im: '/projects/im' },
    im: { listBots: async () => [], listChats: async () => [] },
};

const Where = () => {
    const l = useLocation();
    return <div data-testid="where">{l.pathname + l.search}</div>;
};
const show = (hostUsed: ProjectsHost = host) => render(
    <MemoryRouter initialEntries={['/workspace']}>
        <ProjectsHostProvider host={hostUsed}>
            <Routes><Route path="/workspace" element={<WorkspaceHomePage />} /><Route path="*" element={<Where />} /></Routes>
        </ProjectsHostProvider>
    </MemoryRouter>,
);
const section = (name: string) => within(screen.getByRole('region', { name }));

beforeAll(() => {
    // jsdom has no layout, so scrolling has nothing to do.
    Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => {
    cleanup();
    vi.useRealTimers();
});

describe('WorkspaceHomePage', () => {
    it('guides both sides when the user has nothing yet', () => {
        m.projects = [];
        m.folders = [];
        show();
        for (const [name, prefix] of [['nav.projects', 'projects'], ['workdir.title', 'workdir']] as const) {
            for (const n of [1, 2, 3]) expect(section(name).getByText(`workspace.${prefix}Step${n}`)).toBeInTheDocument();
        }
        expect(screen.queryByText('workspace.viewAllProjects')).not.toBeInTheDocument();
        expect(screen.queryByText('workspace.viewAllWorkdirs')).not.toBeInTheDocument();
    });

    it('carries the notifications control at the top, before every section, and no settings card', () => {
        m.projects = [];
        m.folders = [];
        show();
        const control = screen.getByRole('group', { name: 'notifyControl.label' });
        const sections = screen.getAllByRole('region');
        expect(control.compareDocumentPosition(sections[0]) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
        expect(screen.queryByText('settings.notificationsTitle')).not.toBeInTheDocument();
    });

    it('shows the newest items and drops the guide once there are some', () => {
        m.projects = [1, 2, 3, 4].map(project);
        m.folders = [folder(1)];
        show();
        const projects = section('nav.projects');
        expect(projects.queryByText('workspace.projectsStep1')).not.toBeInTheDocument();
        expect(projects.getAllByRole('button', { name: /Project \d/ })).toHaveLength(RECENT);
        expect(projects.getByText('Project 4')).toBeInTheDocument();
        expect(projects.queryByText('Project 1')).not.toBeInTheDocument();
        expect(projects.getByText('workspace.viewAllProjects')).toBeInTheDocument();
        const workdirs = section('workdir.title');
        expect(workdirs.getByText('app1')).toBeInTheDocument();
        expect(workdirs.queryByText('workspace.workdirStep1')).not.toBeInTheDocument();
    });

    it('guides one side while the other shows cards', () => {
        m.projects = [project(1)];
        m.folders = [];
        show();
        expect(section('nav.projects').getByText('Project 1')).toBeInTheDocument();
        expect(section('workdir.title').getByText('workspace.workdirStep1')).toBeInTheDocument();
    });

    it('shows placeholders until the lists load', () => {
        m.projects = null;
        m.folders = null;
        show();
        expect(screen.queryByText('workspace.projectsStep1')).not.toBeInTheDocument();
        expect(screen.queryByText('workspace.viewAllProjects')).not.toBeInTheDocument();
    });

    it('sends the primary actions and View all to the existing pages', () => {
        m.projects = [project(1)];
        m.folders = [folder(1)];
        const go = (name: string | RegExp) => {
            cleanup();
            show();
            fireEvent.click(screen.getByRole('button', { name }));
            return screen.getByTestId('where').textContent;
        };
        expect(go('projects.new')).toBe('/projects?new=1');
        expect(go('workdir.add')).toBe('/projects/folder?add=1');
        expect(go('workspace.viewAllProjects')).toBe('/projects');
        expect(go('workspace.viewAllWorkdirs')).toBe('/projects/folder');
        expect(go(/^Project 1/)).toBe('/projects/p1');
        expect(go(/^app1/)).toBe('/projects/folder?path=/home/me/app1');
    });
});

describe('WorkspaceHomePage IM section', () => {
    beforeEach(() => {
        window.localStorage.clear();
        window.sessionStorage.clear();
        m.projects = [project(1)];
        m.folders = [folder(1)];
        m.all = [project(1), folder(1)];
        m.bindings = null;
    });

    it('stays away entirely when the host has no IM', () => {
        m.bindings = [];
        show();
        expect(screen.queryByText('workspace.imBody')).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'workspace.imAction' })).not.toBeInTheDocument();
    });

    it('guides IM until a chat is bound, then shows the newest bound chats', async () => {
        m.bindings = [];
        show(hostWithIm);
        await act(async () => {}); // let the bindings load land
        const im = section('im.title');
        for (const n of [1, 2, 3]) expect(im.getByText(`workspace.imStep${n}`)).toBeInTheDocument();
        expect(im.getByRole('button', { name: 'workspace.imAction' })).toBeInTheDocument();
        expect(im.queryByRole('button', { name: /Chat 1/ })).not.toBeInTheDocument();
        cleanup();

        m.bindings = [binding(1), binding(2, { updated_at: '2026-11-05T00:00:00Z' })];
        show(hostWithIm);
        await act(async () => {});
        const line = section('im.title');
        expect(line.queryByText('workspace.imStep1')).not.toBeInTheDocument();
        // One line: the count, then the newest chats first with where they are bound.
        expect(line.getByText(/^workspace\.imBound \{"count":2\} · Chat 2 → .*· Chat 1 → /)).toBeInTheDocument();
        expect(line.queryByRole('button', { name: /Chat/ })).not.toBeInTheDocument();
    });

    it('opens the IM page from the one-line summary', async () => {
        m.bindings = [binding(1)];
        show(hostWithIm);
        await act(async () => {});
        fireEvent.click(section('im.title').getByRole('button', { name: 'workspace.imManage' }));
        expect(screen.getByTestId('where').textContent).toBe('/projects/im');
    });
});

describe('WorkspaceHomePage first-visit IM hint', () => {
    // The hint is armed by enabling the Workspace power-up (the host calls
    // armWorkspaceImHint); here the tests arm it the same way. The host
    // layout also renders the sidebar's sticky IM entry, which the hint
    // anchors to — the tests stub it on the document.
    let sidebarStub: HTMLElement | null = null;
    const stubSidebar = () => {
        sidebarStub = document.createElement('div');
        sidebarStub.id = 'projects-sidebar-im';
        document.body.appendChild(sidebarStub);
    };

    beforeEach(() => {
        window.localStorage.clear();
        window.sessionStorage.clear();
        armWorkspaceImHint();
        m.projects = [project(1)];
        m.folders = [folder(1)];
        m.all = [project(1), folder(1)];
        m.bindings = [];
    });

    afterEach(() => {
        sidebarStub?.remove();
        sidebarStub = null;
    });

    const openHint = (opts: { sidebar?: boolean } = {}) => {
        if (opts.sidebar !== false) stubSidebar();
        vi.useFakeTimers();
        show(hostWithIm);
        act(() => { vi.advanceTimersByTime(opts.sidebar === false ? 4200 : 600); });
    };

    it('stays quiet on visits that follow no fresh enablement', () => {
        window.localStorage.clear();
        vi.useFakeTimers();
        show(hostWithIm);
        act(() => { vi.advanceTimersByTime(600); });
        expect(screen.queryByText('workspace.imHintTitle')).not.toBeInTheDocument();
    });

    it('points at the layout IM entry once after enablement, and a dismissed hint stays away for good', () => {
        openHint();
        expect(screen.getByText('workspace.imHintTitle')).toBeInTheDocument();
        expect(screen.getByText('workspace.imHintBody')).toBeInTheDocument();
        // Shown: the arm is spent.
        expect(window.localStorage.getItem('tingly-project.workspace.imHintArmed')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'workspace.imHintGotIt' }));
        expect(window.localStorage.getItem('tingly-project.workspace.imHintDone')).toBe('1');
        cleanup();

        // Enabling the power-up again does not bring it back.
        armWorkspaceImHint();
        vi.useFakeTimers();
        show(hostWithIm);
        act(() => { vi.advanceTimersByTime(600); });
        expect(screen.queryByText('workspace.imHintTitle')).not.toBeInTheDocument();
    });

    it('falls back to the page IM section when the layout has no IM entry', () => {
        openHint({ sidebar: false });
        expect(screen.getByText('workspace.imHintTitle')).toBeInTheDocument();
    });

    it('leaves the rest of the visit quiet while it is not dismissed', () => {
        openHint();
        expect(screen.getByText('workspace.imHintTitle')).toBeInTheDocument();
        cleanup();

        // A refresh mid-visit: the session flag holds, nothing pops again.
        vi.useFakeTimers();
        show(hostWithIm);
        act(() => { vi.advanceTimersByTime(600); });
        expect(screen.queryByText('workspace.imHintTitle')).not.toBeInTheDocument();
    });

    it('never appears when the host has no IM', () => {
        vi.useFakeTimers();
        show();
        act(() => { vi.advanceTimersByTime(600); });
        expect(screen.queryByText('workspace.imHintTitle')).not.toBeInTheDocument();
    });
});
