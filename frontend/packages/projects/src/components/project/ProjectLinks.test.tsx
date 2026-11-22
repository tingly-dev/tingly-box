import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';
import { ProjectLinks } from './ProjectLinks';

const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('../../client', async (importOriginal) => ({ ...(await importOriginal<typeof import('../../client')>()), api: { POST: post } }));

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.count != null ? `${key}:${o.count}` : o?.title ? `${key}:${o.title}` : key), i18n: { language: 'en' } }),
}));

afterEach(() => {
    post.mockReset();
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
    it('shows the open items, PRs and issues in groups of their own, newest first, each under its thread', () => {
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        const prs = screen.getByRole('region', { name: 'links.prs' });
        expect(within(prs).getAllByRole('link').map((a) => a.textContent)).toEqual(['#2 Newer', '#1 Older']);
        expect(within(prs).getByText('Thread A')).toBeInTheDocument();
        expect(within(prs).getByText('Thread B')).toBeInTheDocument();
        expect(within(screen.getByRole('region', { name: 'links.issues' })).getByRole('link', { name: '#5 Flaky test' })).toBeInTheDocument();
        expect(screen.queryByText('#3 Shipped')).not.toBeInTheDocument();
        expect(screen.queryByText('No links')).not.toBeInTheDocument();
    });

    it('counts open and closed on the switch, and the closed ones hold merged and closed items', () => {
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'links.filterOpen:3' })).toHaveAttribute('aria-pressed', 'true');
        fireEvent.click(screen.getByRole('button', { name: 'links.filterClosed:1' }));
        expect(within(screen.getByRole('region', { name: 'links.prs' })).getByRole('link', { name: '#3 Shipped' })).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'links.issues' })).not.toBeInTheDocument();
        expect(screen.queryByText('#2 Newer')).not.toBeInTheDocument();
    });

    it('opens the thread an item came from', () => {
        const onOpen = vi.fn();
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={onOpen} />);
        fireEvent.click(screen.getByRole('button', { name: 'Thread B' }));
        expect(onOpen).toHaveBeenCalledWith('b');
    });

    it('says what it is for when no thread touched anything', () => {
        render(<ProjectLinks projectId="p1" threads={[thread([])]} onOpen={vi.fn()} />);
        expect(screen.getByText('links.empty')).toBeInTheDocument();
    });

    it('opens on the finished items when nothing is open, and says so when the user then asks for the open ones', () => {
        render(<ProjectLinks projectId="p1" threads={[thread([link({ state: 'merged' })])]} onOpen={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'links.filterClosed:1' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('link', { name: '#12 Add the tool' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'links.filterOpen:0' }));
        expect(screen.getByText('links.noneOpen')).toBeInTheDocument();
    });

    it('refreshes every open item from GitHub in one request, and says what it did', async () => {
        post.mockResolvedValue({ data: { results: [{ repo: 'acme/w', kind: 'pr', number: 1, status: 'refreshed' }, { repo: 'acme/w', kind: 'pr', number: 2, status: 'failed', error: 'boom' }] }, response: { ok: true } });
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        fireEvent.click(screen.getByRole('button', { name: 'links.refreshAll' }));
        expect(post).toHaveBeenCalledWith('/api/v1/projects/{id}/links/refresh', { params: { path: { id: 'p1' } } });
        await waitFor(() => expect(screen.getByText('links.refreshedSome')).toBeInTheDocument());
        expect(screen.getByRole('button', { name: 'links.refreshAll' })).toBeEnabled();
    });

    it('has nothing to refresh all of when nothing is open, and a row refreshes through the thread it belongs to', () => {
        render(<ProjectLinks projectId="p1" threads={[thread([link({ state: 'merged' })])]} onOpen={vi.fn()} />);
        expect(screen.queryByRole('button', { name: 'links.refreshAll' })).not.toBeInTheDocument();
    });

    it('sends a row\'s refresh to the thread that owns the item', async () => {
        post.mockResolvedValue({ data: { results: [{ repo: 'acme/w', kind: 'pr', number: 2, status: 'refreshed' }] }, response: { ok: true } });
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        let row = screen.getByRole('link', { name: '#2 Newer' }) as HTMLElement;
        while (!within(row).queryByRole('button', { name: 'links.refresh' })) row = row.parentElement!;
        fireEvent.click(within(row).getByRole('button', { name: 'links.refresh' }));
        expect(post).toHaveBeenCalledWith('/api/v1/threads/{id}/links/refresh', { params: { path: { id: 'b' } }, body: { repo: 'acme/w', kind: 'pr', number: 2 } });
    });

    it('folds a group from its heading and remembers it', () => {
        const { unmount } = render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        const heading = screen.getByRole('button', { name: 'links.prs 2' });
        expect(heading).toHaveAttribute('aria-expanded', 'true');
        fireEvent.click(heading);
        expect(heading).toHaveAttribute('aria-expanded', 'false');
        unmount();
        render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'links.prs 2' })).toHaveAttribute('aria-expanded', 'false');
        expect(screen.getByRole('button', { name: 'links.issues 1' })).toHaveAttribute('aria-expanded', 'true');
    });

    it('starts the other list at its top, and keeps the place when the threads update', () => {
        const { container, rerender } = render(<ProjectLinks projectId="p1" threads={mixed()} onOpen={vi.fn()} />);
        const scroller = [...container.querySelectorAll<HTMLElement>('div')].find((d) => getComputedStyle(d).overflowY === 'auto')!;
        scroller.scrollTop = 120;
        rerender(<ProjectLinks projectId="p1" threads={[...mixed()]} onOpen={vi.fn()} />);
        expect(scroller.scrollTop).toBe(120);
        fireEvent.click(screen.getByRole('button', { name: 'links.filterClosed:1' }));
        expect(scroller.scrollTop).toBe(0);
    });
});
