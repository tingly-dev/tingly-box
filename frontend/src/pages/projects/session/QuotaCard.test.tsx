import {cleanup, render, screen, waitFor} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import type {Thread} from '@tingly/projects';
import QuotaCard from './QuotaCard';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({t: (key: string, o?: {defaultValue?: string}) => o?.defaultValue ?? key}),
}));

const getClaudeCodeRoute = vi.hoisted(() => vi.fn());
vi.mock('@/services/profileApi', () => ({profileApi: {getClaudeCodeRoute}}));

afterEach(() => {
    cleanup();
    localStorage.clear();
    getClaudeCodeRoute.mockReset();
});

const thread = (over: Partial<Thread> = {}) => ({id: 't1', turns: 1, reported_model: 'sonnet', ...over}) as Thread;
const launch = {profile: 'work', model: 'sonnet'};
const hours = (h: number) => new Date(Date.now() + h * 3_600_000).toISOString();
const route = {
    scenario: 'claude_code:work', provider_name: 'zai', provider_model: 'glm-4.6',
    quota: [
        {type: 'session', balance: false, text: '14% left', used_percent: 86, resets_at: hours(2), limit_reached: false},
        {type: 'weekly', balance: false, text: '81% left', used_percent: 19, resets_at: hours(80), limit_reached: false},
    ],
};

describe('QuotaCard', () => {
    it("shows the routed provider's windows as what is left, looked up by the thread's launch", async () => {
        getClaudeCodeRoute.mockResolvedValue(route);
        render(<QuotaCard thread={thread()} launch={launch}/>);
        expect(await screen.findByText('14% left')).toBeTruthy();
        expect(screen.getByText('81% left')).toBeTruthy();
        expect(screen.getByText('glm-4.6 @ zai')).toBeTruthy();
        // The bar shrinks as the window is spent.
        expect(screen.getByRole('progressbar', {name: 'session'}).getAttribute('aria-valuenow')).toBe('14');
        expect(getClaudeCodeRoute).toHaveBeenCalledWith({profile: 'work', tier: 'sonnet', model: 'sonnet'});
    });

    it('shows a balance as its amount, without a bar', async () => {
        getClaudeCodeRoute.mockResolvedValue({...route, quota: [{type: 'balance', balance: true, text: '$12.40', used_percent: 0, limit_reached: false}]});
        render(<QuotaCard thread={thread()} launch={launch}/>);
        expect(await screen.findByText('$12.40')).toBeTruthy();
        expect(screen.queryByRole('progressbar')).toBeNull();
    });

    it('turns red once the limit is reached', async () => {
        getClaudeCodeRoute.mockResolvedValue({...route, quota: [{type: 'daily', balance: false, text: '0% left', used_percent: 100, limit_reached: true}]});
        render(<QuotaCard thread={thread()} launch={launch}/>);
        const bar = await screen.findByRole('progressbar');
        expect(bar.getAttribute('aria-valuenow')).toBe('0');
    });

    it('asks again after each turn', async () => {
        getClaudeCodeRoute.mockResolvedValue(route);
        const {rerender} = render(<QuotaCard thread={thread({last_turn_at: '2026-11-06T10:00:00Z'})} launch={launch}/>);
        await waitFor(() => expect(getClaudeCodeRoute).toHaveBeenCalledTimes(1));
        rerender(<QuotaCard thread={thread({turns: 2, last_turn_at: '2026-11-06T10:05:00Z'})} launch={launch}/>);
        await waitFor(() => expect(getClaudeCodeRoute).toHaveBeenCalledTimes(2));
    });

    it('is no card when there is no route or the provider has no quota', async () => {
        getClaudeCodeRoute.mockResolvedValue(undefined);
        const {container, rerender} = render(<QuotaCard thread={thread()} launch={launch}/>);
        await waitFor(() => expect(getClaudeCodeRoute).toHaveBeenCalled());
        expect(container.textContent).toBe('');
        getClaudeCodeRoute.mockResolvedValue({...route, quota: []});
        rerender(<QuotaCard thread={thread({id: 't2'})} launch={launch}/>);
        await waitFor(() => expect(getClaudeCodeRoute).toHaveBeenCalledTimes(2));
        expect(container.textContent).toBe('');
    });

    it("never shows another thread's reading while the new one loads", async () => {
        getClaudeCodeRoute.mockResolvedValueOnce(route).mockReturnValueOnce(new Promise(() => undefined));
        const {rerender} = render(<QuotaCard thread={thread({id: 'a'})} launch={launch}/>);
        expect(await screen.findByText('14% left')).toBeTruthy();
        rerender(<QuotaCard thread={thread({id: 'b'})} launch={launch}/>);
        expect(screen.queryByText('14% left')).toBeNull();
    });
});
