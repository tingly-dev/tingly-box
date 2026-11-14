import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';
import { ProjectLinks } from './ProjectLinks';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.count != null ? `${key}:${o.count}` : o?.title ? `${key}:${o.title}` : key), i18n: { language: 'en' } }),
}));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const now = new Date().toISOString();
const link = (o: Partial<ThreadLink>): ThreadLink => ({
    kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool',
    role: 'opened', state: 'open', refreshed_at: now, attempted_at: now, created_at: now, ...o,
} as ThreadLink);
const thread = (links: ThreadLink[], o: Partial<Thread> = {}): Thread => ({ id: 't1', title: 'Fix export', links, ...o } as Thread);

const mixed = () => {
    const old = new Date(Date.now() - 3600_000).toISOString();
    return [
        thread([link({ number: 1, title: 'Older', created_at: old }), link({ kind: 'issue', number: 5, title: 'Flaky test', url: 'https://github.com/acme/w/issues/5' })], { id: 'a', title: 'Thread A' }),
        thread([link({ number: 2, title: 'Newer' }), link({ number: 3, title: 'Shipped', state: 'merged' })], { id: 'b', title: 'Thread B' }),
        thread([], { id: 'c', title: 'No links' }),
    ];
};

describe('ProjectLinks', () => {
    it('shows the open items, PRs and issues in cards of their own, newest first, each under its thread', () => {
        render(<ProjectLinks threads={mixed()} onOpen={vi.fn()} />);
        const prs = screen.getByRole('region', { name: 'links.prs' });
        expect(within(prs).getAllByRole('link').map((a) => a.textContent)).toEqual(['#2 Newer', '#1 Older']);
        expect(within(prs).getByText('Thread A')).toBeInTheDocument();
        expect(within(prs).getByText('Thread B')).toBeInTheDocument();
        expect(within(screen.getByRole('region', { name: 'links.issues' })).getByRole('link', { name: '#5 Flaky test' })).toBeInTheDocument();
        expect(screen.queryByText('#3 Shipped')).not.toBeInTheDocument();
        expect(screen.queryByText('No links')).not.toBeInTheDocument();
    });

    it('counts open and closed on the switch, and the closed ones hold merged and closed items', () => {
        render(<ProjectLinks threads={mixed()} onOpen={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'links.filterOpen:3' })).toHaveAttribute('aria-pressed', 'true');
        fireEvent.click(screen.getByRole('button', { name: 'links.filterClosed:1' }));
        expect(within(screen.getByRole('region', { name: 'links.prs' })).getByRole('link', { name: '#3 Shipped' })).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'links.issues' })).not.toBeInTheDocument();
        expect(screen.queryByText('#2 Newer')).not.toBeInTheDocument();
    });

    it('opens the thread an item came from', () => {
        const onOpen = vi.fn();
        render(<ProjectLinks threads={mixed()} onOpen={onOpen} />);
        fireEvent.click(screen.getByRole('button', { name: 'Thread B' }));
        expect(onOpen).toHaveBeenCalledWith('b');
    });

    it('says what it is for when no thread touched anything, and when nothing is open', () => {
        const { unmount } = render(<ProjectLinks threads={[thread([])]} onOpen={vi.fn()} />);
        expect(screen.getByText('links.empty')).toBeInTheDocument();
        unmount();
        render(<ProjectLinks threads={[thread([link({ state: 'merged' })])]} onOpen={vi.fn()} />);
        expect(screen.getByText('links.noneOpen')).toBeInTheDocument();
    });
});
