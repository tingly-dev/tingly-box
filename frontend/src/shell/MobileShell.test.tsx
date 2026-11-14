import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ActivityItem } from '@/layout/types';

const { flags } = vi.hoisted(() => ({ flags: { enableProjects: true } }));
const items: ActivityItem[] = [
    { key: 'scenario', icon: <i>A</i>, label: 'Agent', defaultPath: '/agent', children: [{ path: '/agent/claude_code', label: 'Claude Code' }] },
    { key: 'projects', icon: <i>W</i>, label: 'Workspace', defaultPath: '/workspace', attention: 'waiting', children: [{ path: '/workspace', label: 'Workspace', match: (p) => p === '/workspace' }] },
    { key: 'credential', icon: <i>C</i>, label: 'Credentials', defaultPath: '/credentials', path: '/credentials' },
];
vi.mock('@/layout/useActivityItems', () => ({ useActivityItems: () => items }));
vi.mock('@/contexts/FeatureFlagsContext', () => ({ useFeatureFlags: () => flags }));
vi.mock('@/services/uiPrefs', () => ({ syncUiPrefs: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import MobileShell from './MobileShell';

const at = (path: string) => render(
    <MemoryRouter initialEntries={[path]}>
        <Routes><Route element={<MobileShell />}><Route path="*" element={<p>page</p>} /></Route></Routes>
    </MemoryRouter>,
);
afterEach(() => { cleanup(); flags.enableProjects = true; });

describe('MobileShell', () => {
    it('frames the page with the Workspace, Agent and More tabs', () => {
        at('/credentials');
        const nav = screen.getByRole('navigation', { name: 'mobile.tabBar' });
        expect(nav).toHaveTextContent('Workspace');
        expect(nav).toHaveTextContent('Agent');
        expect(nav).toHaveTextContent('mobile.more.tab');
        expect(screen.getByText('page')).toBeInTheDocument();
    });

    it('marks the tab of the current page, and More for pages that belong to neither', () => {
        at('/credentials');
        expect(screen.getByRole('link', { name: /mobile.more.tab/ })).toHaveClass('Mui-selected');
        cleanup();
        at('/workspace');
        expect(screen.getByRole('link', { name: /Workspace/ })).toHaveClass('Mui-selected');
    });

    it('drops the Workspace tab while Projects is off', () => {
        flags.enableProjects = false;
        at('/agent/claude_code');
        expect(screen.queryByRole('link', { name: /Workspace/ })).not.toBeInTheDocument();
        expect(screen.getByRole('link', { name: /Agent/ })).toHaveClass('Mui-selected');
    });
});
