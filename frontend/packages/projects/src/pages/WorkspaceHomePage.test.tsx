import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import WorkspaceHomePage, { RECENT } from './WorkspaceHomePage';

const m = vi.hoisted(() => ({ projects: null as unknown[] | null, folders: null as unknown[] | null }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key), i18n: { language: 'en' } }),
}));
vi.mock('../hooks/useProjectList', () => ({
    useProjectList: () => ({ projects: m.projects, folders: m.folders, all: null, waiting: 0, reload: vi.fn() }),
}));

const threads = { working: 0, waiting: 0, failed: 0, ready: 0, idle: 0, resolved: 0 };
const project = (n: number) => ({
    id: `p${n}`, name: `Project ${n}`, type: 'project', goal: '', starred: false, paused: false, repos: [], threads,
    updated_at: `2026-11-0${n}T00:00:00Z`,
});
const folder = (n: number) => ({
    id: `f${n}`, name: `app${n}`, path: `/home/me/app${n}`, type: 'folder', starred: false, paused: false, repos: [], threads,
    updated_at: `2026-11-0${n}T00:00:00Z`,
});
const host: ProjectsHost = {
    paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: (p) => (p ? `/projects/folder?path=${p}` : '/projects/folder') },
};

const Where = () => {
    const l = useLocation();
    return <div data-testid="where">{l.pathname + l.search}</div>;
};
const show = () => render(
    <MemoryRouter initialEntries={['/workspace']}>
        <ProjectsHostProvider host={host}>
            <Routes><Route path="/workspace" element={<WorkspaceHomePage />} /><Route path="*" element={<Where />} /></Routes>
        </ProjectsHostProvider>
    </MemoryRouter>,
);
const section = (name: string) => within(screen.getByRole('region', { name }));

afterEach(cleanup);

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
