import { cleanup, fireEvent, render, renderHook, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ActivityItem, NavItemBase } from '@/layout/types';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: { defaultValue?: string }) => o?.defaultValue ?? key }),
}));
vi.mock('@/contexts/FeatureFlagsContext', () => ({
    useFeatureFlags: () => ({ skillIde: true, enableGuardrails: true, enableBench: true, enableProjects: true }),
}));
vi.mock('@/layout/useProjectsWaiting', () => ({ useProjectsWaiting: () => false }));
vi.mock('@/contexts/ProfileContext', () => ({ useProfileContext: () => ({ profiles: { claude_code: [{ id: 'p1', name: 'ds' }, { id: 'p2', name: 'sonnet' }] } }) }));
vi.mock('@/pages/mcp/profileStore', () => ({ useMCPProfiles: () => [{ id: 'default', name: 'Default' }, { id: 'm2', name: 'Extra' }] }));
vi.mock('@/pages/image/profiles/imageProfileStore', () => ({ useImageProfiles: () => [{ id: 'i1', name: 'Poster', refs: [] }] }));
vi.mock('@/contexts/TeamContext', () => ({ useTeamContext: () => ({ teams: [{ slug: 'a', name: 'Alpha', is_default: true }, { slug: 'b', name: 'Beta' }] }) }));
vi.mock('@/services/uiPrefs', () => ({ syncUiPrefs: vi.fn() }));

import { useActivityItems } from '@/layout/useActivityItems';
import MobileShell from './MobileShell';
import { hasSubPages, subPageGroups } from './mobileSubPages';

const pathsOf = (item: ActivityItem) =>
    (item.children ?? []).filter((c): c is NavItemBase => c.type !== 'divider' && !c.path.startsWith('#') && !c.hidden).map((c) => c.path);

const activities = () => renderHook(() => useActivityItems()).result.current.filter((i) => !i.sidebar && pathsOf(i).length > 1);

afterEach(cleanup);

describe('phone sub-pages', () => {
    it('cover every page the desktop sidebar lists, for every activity', () => {
        const list = activities();
        // Agent, Team, Image, Dashboard, Power-ups and System all have several pages.
        expect(list.map((i) => i.key)).toEqual(expect.arrayContaining(['scenario', 'team', 'image', 'dashboard', 'powerups', 'system']));
        for (const item of list) {
            for (const path of pathsOf(item)) {
                const pages = subPageGroups(item, path).flat();
                expect(pages.map((p) => p.path), `${item.key} lists ${path}`).toContain(path);
                expect(pages.filter((p) => p.active).map((p) => p.path), `${item.key} at ${path}`).toEqual([path]);
            }
        }
    });

    it('leaves out "add" actions and hidden agents, and has none for the Workspace', () => {
        const items = renderHook(() => useActivityItems()).result.current;
        const agent = items.find((i) => i.key === 'scenario')!;
        agent.children = [...agent.children!, { path: '/agent/ghost', label: 'Ghost', hidden: true }];
        const paths = subPageGroups(agent, '/agent/claude_code').flat().map((p) => p.path);
        expect(paths).not.toContain('#add-profile');
        expect(paths).not.toContain('/agent/ghost');
        expect(hasSubPages(subPageGroups(items.find((i) => i.key === 'projects'), '/workspace'))).toBe(false);
    });

    it('shows a switcher in the shell whose sheet links every page of the activity', () => {
        for (const item of activities()) {
            const paths = pathsOf(item);
            render(
                <MemoryRouter initialEntries={[paths[0]]}>
                    <Routes><Route element={<MobileShell />}><Route path="*" element={<p>page</p>} /></Route></Routes>
                </MemoryRouter>,
            );
            fireEvent.click(screen.getByRole('button', { name: 'mobile.subPages.switch' }));
            const sheet = screen.getByRole('dialog');
            const hrefs = within(sheet).getAllByRole('link').map((a) => a.getAttribute('href'));
            expect(hrefs, item.key).toEqual(paths);
            cleanup();
        }
    });

    it('has no switcher on the Workspace, on More, or on a single-page activity', () => {
        for (const path of ['/workspace', '/more', '/credentials']) {
            render(
                <MemoryRouter initialEntries={[path]}>
                    <Routes><Route element={<MobileShell />}><Route path="*" element={<p>page</p>} /></Route></Routes>
                </MemoryRouter>,
            );
            expect(screen.queryByRole('button', { name: 'mobile.subPages.switch' }), path).not.toBeInTheDocument();
            cleanup();
        }
    });
});
