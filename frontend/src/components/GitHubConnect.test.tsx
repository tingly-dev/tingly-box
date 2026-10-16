import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import * as api from '@/services/githubApi';
import GitHubConnect from './GitHubConnect';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (_k: string, o: Record<string, unknown> & {defaultValue: string}) =>
            o.defaultValue.replace(/\{\{(\w+)\}\}/g, (_m, name: string) => String(o[name])),
    }),
}));
vi.mock('@/services/githubApi', () => ({
    getGitHubStatus: vi.fn(), startGitHubConnect: vi.fn(), submitManifest: vi.fn(), syncGitHub: vi.fn(),
    disconnectGitHub: vi.fn(), setGitHubToken: vi.fn(), clearGitHubToken: vi.fn(),
}));

afterEach(cleanup);

const status = (over: Partial<api.GitHubStatus> = {}): api.GitHubStatus =>
    ({connected: false, has_pat: false, installations: [], ...over});

describe('GitHubConnect', () => {
    it('starts the manifest flow for a personal account', async () => {
        vi.mocked(api.getGitHubStatus).mockResolvedValue(status());
        const plan = {action_url: 'https://github.com/settings/apps/new?state=s', manifest: '{}', state: 's'};
        vi.mocked(api.startGitHubConnect).mockResolvedValue(plan);

        render(<GitHubConnect/>);
        fireEvent.click(await screen.findByText('Connect GitHub'));

        await waitFor(() => expect(api.submitManifest).toHaveBeenCalledWith(plan));
        expect(api.startGitHubConnect).toHaveBeenCalledWith('user', '');
    });

    it('shows the installed accounts once connected', async () => {
        vi.mocked(api.getGitHubStatus).mockResolvedValue(status({
            connected: true, app_slug: 'tb-me', app_owner: 'me', install_url: 'https://github.com/apps/tb-me/installations/new',
            installations: [{id: 1, account: 'me', account_type: 'User', repository_selection: 'selected', repositories: 3}],
        }));

        render(<GitHubConnect/>);

        expect(await screen.findByText(/me: 3 repositories/)).toBeTruthy();
        expect(screen.queryByText('Connect GitHub')).toBeNull();
    });
});
