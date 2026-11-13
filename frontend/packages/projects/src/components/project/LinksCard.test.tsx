import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';
import { ProjectLinksCard, ThreadLinksCard } from './LinksCard';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.age ? `${key} ${o.age}` : key), i18n: { language: 'en' } }),
}));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const now = new Date().toISOString();
const link = (o: Partial<ThreadLink>): ThreadLink => ({
    thread_id: 't1', project_id: 'p', kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool',
    role: 'opened', state: 'open', refreshed_at: now, created_at: now, updated_at: now, ...o,
} as ThreadLink);
const thread = (links: ThreadLink[], o: Partial<Thread> = {}): Thread => ({ id: 't1', title: 'Fix export', links, ...o } as Thread);

describe('ThreadLinksCard', () => {
    it('is absent when the thread touched no issue or PR', () => {
        render(<ThreadLinksCard thread={thread([])} />);
        expect(screen.queryByRole('region', { name: 'links.title' })).not.toBeInTheDocument();
    });

    it('lists each item with its link, where it stands and its checks', () => {
        render(<ThreadLinksCard thread={thread([
            link({ ci: 'failing', merge_state: 'conflicts' }),
            link({ kind: 'issue', number: 3, title: 'Flaky test', url: 'https://github.com/acme/w/issues/3' }),
        ])} />);
        const card = screen.getByRole('region', { name: 'links.title' });
        const pr = within(card).getByRole('link', { name: '#12 Add the tool' });
        expect(pr).toHaveAttribute('href', 'https://github.com/acme/w/pull/12');
        expect(pr).toHaveAttribute('target', '_blank');
        expect(pr).toHaveAttribute('rel', expect.stringContaining('noopener'));
        expect(within(card).getByText('links.checksFailing')).toBeInTheDocument();
        expect(within(card).getByText('links.conflicts')).toBeInTheDocument();
        expect(within(card).getByRole('link', { name: '#3 Flaky test' })).toBeInTheDocument();
    });

    it('shows a merged PR as merged without checks, and a draft as a draft', () => {
        render(<ThreadLinksCard thread={thread([
            link({ state: 'merged', ci: 'failing' }),
            link({ number: 13, title: 'WIP', draft: true }),
        ])} />);
        expect(screen.getByText('links.merged')).toBeInTheDocument();
        expect(screen.queryByText('links.checksFailing')).not.toBeInTheDocument();
        expect(screen.getByText('links.draft')).toBeInTheDocument();
    });

    it('leaves out what GitHub was not asked about yet, and says when a refresh failed', () => {
        render(<ThreadLinksCard thread={thread([link({}), link({ number: 14, title: 'Old', last_error: 'HTTP 403' })])} />);
        expect(screen.queryByText('links.checksPassing')).not.toBeInTheDocument();
        expect(screen.getByText(/links\.stale/)).toBeInTheDocument();
    });

    it('never makes a link of an address that is not https', () => {
        render(<ThreadLinksCard thread={thread([link({ url: 'javascript:alert(1)' })])} />);
        expect(screen.getByText('#12 Add the tool').closest('a')).not.toHaveAttribute('href');
    });
});

describe('ProjectLinksCard', () => {
    it('gathers every thread\'s items, newest first, each under its thread', () => {
        const old = new Date(Date.now() - 3600_000).toISOString();
        render(<ProjectLinksCard threads={[
            thread([link({ number: 1, title: 'Older', created_at: old })], { id: 'a', title: 'Thread A' }),
            thread([link({ number: 2, title: 'Newer', state: 'merged' })], { id: 'b', title: 'Thread B' }),
            thread([], { id: 'c', title: 'No links' }),
        ]} />);
        const card = screen.getByRole('region', { name: 'links.title' });
        const rows = within(card).getAllByRole('link').map((a) => a.textContent);
        expect(rows).toEqual(['#2 Newer', '#1 Older']);
        expect(within(card).getByText('Thread A')).toBeInTheDocument();
        expect(within(card).getByText('Thread B')).toBeInTheDocument();
        expect(within(card).queryByText('No links')).not.toBeInTheDocument();
    });

    it('is absent when no thread has any', () => {
        render(<ProjectLinksCard threads={[thread([])]} />);
        expect(screen.queryByRole('region', { name: 'links.title' })).not.toBeInTheDocument();
    });
});
