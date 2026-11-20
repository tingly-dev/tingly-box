import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, expect, it, vi } from 'vitest';
import type { ProjectSummary } from '../../client';
import { ProjectsHostProvider, type ProjectsHost } from '../../host/ProjectsHost';
import { ProjectCard } from './ProjectCard';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}));

afterEach(cleanup);

const host: ProjectsHost = {
    paths: { list: '/projects', project: (id) => `/projects/${id}`, folder: (p) => (p ? `/projects/folder?path=${p}` : '/projects/folder') },
};
const threads = { working: 0, waiting: 0, failed: 0, ready: 0, idle: 0, resolved: 0 };
const summary = (repos: unknown): ProjectSummary => ({
    id: 'p1', name: 'Docs', type: 'project', goal: '', starred: false, paused: false, repos, threads, updated_at: '2026-11-01T00:00:00Z',
} as ProjectSummary);
const show = (p: ProjectSummary) => render(
    <MemoryRouter><ProjectsHostProvider host={host}><ProjectCard p={p} /></ProjectsHostProvider></MemoryRouter>,
);

it('shows the directories a project works in', () => {
    show(summary([{ path: '/home/me/app', base_branch: 'main', kind: 'git' }, { path: '/home/me/docs', base_branch: '', kind: 'directory' }]));
    expect(screen.getByText('app · docs')).toBeTruthy();
});

it('renders a project whose repos come back null, instead of crashing the page', () => {
    // A project with no repositories used to be sent as `"repos": null`.
    show(summary(null));
    expect(screen.getByText('Docs')).toBeTruthy();
});
