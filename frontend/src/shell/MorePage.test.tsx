import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ActivityItem } from '@/layout/types';

const items: ActivityItem[] = [
    { key: 'scenario', icon: null, label: 'Agent', defaultPath: '/agent' },
    { key: 'projects', icon: null, label: 'Workspace', defaultPath: '/workspace' },
    { key: 'team', icon: <i>T</i>, label: 'Team', defaultPath: '/agent/team' },
    { key: 'credential', icon: <i>C</i>, label: 'Credentials', defaultPath: '/credentials' },
    { key: 'system', icon: <i>S</i>, label: 'System', defaultPath: '/system', railHidden: true },
];
vi.mock('@/layout/useActivityItems', () => ({ useActivityItems: () => items }));
vi.mock('@/layout/PreferencesMenu', () => ({ PreferencesMenu: ({ anchorEl }: { anchorEl: HTMLElement | null }) => (anchorEl ? <p>prefs open</p> : null) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

const load = async () => (await import('./MorePage')).default;
const shell = async () => import('./uiShell');

const renderAt = async () => {
    const MorePage = await load();
    return render(
        <MemoryRouter initialEntries={['/more']}>
            <Routes>
                <Route path="/more" element={<MorePage />} />
                <Route path="/agent" element={<p>agent page</p>} />
            </Routes>
        </MemoryRouter>,
    );
};

beforeEach(() => { vi.resetModules(); localStorage.clear(); window.history.replaceState({}, '', '/'); });
afterEach(cleanup);

describe('MorePage', () => {
    it('sends the desktop shell to the Agent page', async () => {
        await renderAt();
        expect(screen.getByText('agent page')).toBeInTheDocument();
    });

    it('lists what is neither Workspace nor Agent, then system, preferences and help', async () => {
        (await shell()).setShellPref('mobile');
        await renderAt();
        expect(screen.getByRole('link', { name: /Team/ })).toHaveAttribute('href', '/agent/team');
        expect(screen.getByRole('link', { name: /Credentials/ })).toHaveAttribute('href', '/credentials');
        expect(screen.getByRole('link', { name: /System/ })).toHaveAttribute('href', '/system');
        expect(screen.getByRole('link', { name: /layout.help/ })).toHaveAttribute('href', '/help');
        expect(screen.queryByRole('link', { name: /Agent/ })).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: /Workspace/ })).not.toBeInTheDocument();
    });

    it('opens the preferences menu from its row', async () => {
        (await shell()).setShellPref('mobile');
        await renderAt();
        fireEvent.click(screen.getByRole('button', { name: /mobile.more.preferences/ }));
        expect(screen.getByText('prefs open')).toBeInTheDocument();
    });

    it('switches back to the desktop UI', async () => {
        const s = await shell();
        s.setShellPref('mobile');
        await renderAt();
        fireEvent.click(screen.getByRole('button', { name: /mobile.more.useDesktop/ }));
        expect(localStorage.getItem('ui.shell')).toBe('desktop');
        expect(screen.getByText('agent page')).toBeInTheDocument();
    });
});
