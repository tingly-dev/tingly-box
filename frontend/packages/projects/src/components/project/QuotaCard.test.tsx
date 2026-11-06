import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ProjectsHostProvider, type ProjectsHost, type QuotaReading, type SessionSlots } from '../../host/ProjectsHost';
import { QuotaCard } from './infoRail';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, o?: Record<string, unknown>) => (o?.pct != null ? `${o.pct}% left` : key),
        i18n: { language: 'en' },
    }),
}));
afterEach(() => {
    cleanup();
    localStorage.clear();
});

const inHours = (h: number) => Math.floor(Date.now() / 1000) + h * 3600;
const thread = (over: Partial<Thread> = {}): Thread => ({ id: 't1', turns: 1, reported_model: 'sonnet', ...over }) as Thread;
const host = (quota?: SessionSlots['quota']): ProjectsHost => ({
    paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f' },
    session: quota ? { quota } : undefined,
});
const show = (t: Thread, h: ProjectsHost, route?: { profile: string; tier: string }) =>
    render(<ProjectsHostProvider host={h}><QuotaCard thread={t} route={route} /></ProjectsHostProvider>);

describe('QuotaCard', () => {
    it('shows the subscription windows Claude Code reported, as what is left', () => {
        show(thread({ rate_limits: { status: 'allowed', updated_at: '', five_hour: { utilization: 0.42, resets_at: inHours(2) }, seven_day: { utilization: 0.17, resets_at: inHours(100) } } }), host());
        expect(screen.getByRole('region', { name: 'rail.quota' })).toBeTruthy();
        expect(screen.getByText('rail.quotaWindow.five_hour')).toBeTruthy();
        expect(screen.getByText('58% left')).toBeTruthy();
        expect(screen.getByText('83% left')).toBeTruthy();
        expect(screen.getByRole('progressbar', { name: 'rail.quotaWindow.five_hour' }).getAttribute('aria-valuenow')).toBe('58');
    });

    it("shows the quota of the provider the host routes to, looked up by the thread's launch", async () => {
        const reading: QuotaReading = { provider: 'zai', model: 'glm-4.6', windows: [{ label: 'weekly', text: '60% left', usedPct: 40, resetsAt: new Date(Date.now() + 3600_000).toISOString() }] };
        const quota = vi.fn().mockResolvedValue(reading);
        show(thread(), host(quota), { profile: 'work', tier: 'sonnet' });
        expect(await screen.findByText('60% left')).toBeTruthy();
        expect(screen.getByText('glm-4.6 @ zai')).toBeTruthy();
        expect(quota).toHaveBeenCalledWith({ profile: 'work', tier: 'sonnet', model: 'sonnet' });
    });

    it('asks again after each turn', async () => {
        const quota = vi.fn().mockResolvedValue({ windows: [{ label: 'weekly', text: '60% left', usedPct: 40 }] });
        const h = host(quota);
        const { rerender } = show(thread({ turns: 1 }), h, { profile: '', tier: '' });
        await waitFor(() => expect(quota).toHaveBeenCalledTimes(1));
        rerender(<ProjectsHostProvider host={h}><QuotaCard thread={thread({ turns: 2 })} route={{ profile: '', tier: '' }} /></ProjectsHostProvider>);
        await waitFor(() => expect(quota).toHaveBeenCalledTimes(2));
    });

    it('is absent when neither the account nor the host has quota to show', async () => {
        const quota = vi.fn().mockResolvedValue(undefined);
        const { container } = show(thread(), host(quota), { profile: '', tier: '' });
        await waitFor(() => expect(quota).toHaveBeenCalled());
        expect(container.textContent).toBe('');
    });

    it('marks an exhausted window', async () => {
        const quota = vi.fn().mockResolvedValue({ windows: [{ label: 'daily', text: '0% left', usedPct: 100, limitReached: true }] });
        show(thread(), host(quota), { profile: '', tier: '' });
        const bar = await screen.findByRole('progressbar', { name: 'daily' });
        expect(bar.className).toContain('MuiLinearProgress-colorError');
    });

    it("does not show another thread's provider quota once the open thread has no launch to look up", async () => {
        const quota = vi.fn().mockResolvedValue({ provider: 'zai', windows: [{ label: 'weekly', text: '60% left', usedPct: 40 }] });
        const h = host(quota);
        const { rerender } = show(thread({ id: 'a' }), h, { profile: 'work', tier: 'sonnet' });
        expect(await screen.findByText('60% left')).toBeTruthy();
        rerender(<ProjectsHostProvider host={h}><QuotaCard thread={thread({ id: 'b' })} /></ProjectsHostProvider>);
        expect(screen.queryByText('60% left')).toBeNull();
    });

    it('drops a subscription window the moment it resets, without waiting for the next turn', () => {
        vi.useFakeTimers();
        try {
            vi.setSystemTime(new Date('2026-11-06T10:00:00Z'));
            const resetsAt = Math.floor(Date.now() / 1000) + 60;
            show(thread({ rate_limits: { status: 'allowed', updated_at: '', five_hour: { utilization: 0.88, resets_at: resetsAt } } }), host());
            expect(screen.getByText('12% left')).toBeTruthy();
            act(() => { vi.advanceTimersByTime(61_000); });
            expect(screen.queryByText('12% left')).toBeNull();
        } finally {
            vi.useRealTimers();
        }
    });
});
