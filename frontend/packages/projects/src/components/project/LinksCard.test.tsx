import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread, ThreadLink } from '../../client';

const m = vi.hoisted(() => ({ get: vi.fn() }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { GET: m.get, POST: vi.fn() }, unwrap: (p: Promise<unknown>) => p }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.age ? `${key} ${o.age}` : o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }),
}));
import { ThreadIssuesCard, ThreadPrsCard } from './LinksCard';

afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    localStorage.clear();
});

const now = new Date().toISOString();
const link = (o: Partial<ThreadLink>): ThreadLink => ({
    kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool', head: 'tb/fix', base: 'main',
    role: 'opened', state: 'open', refreshed_at: now, attempted_at: now, created_at: now, ...o,
} as ThreadLink);
const issue = (o: Partial<ThreadLink> = {}) => link({ kind: 'issue', number: 3, title: 'Flaky test', url: 'https://github.com/acme/w/issues/3', head: undefined, base: undefined, ...o });
const thread = (links: ThreadLink[], o: Partial<Thread> = {}): Thread => ({ id: 't1', title: 'Fix export', branch: 'tb/fix', base_branch: 'main', commits_ahead: 0, files_changed: 0, links, ...o } as Thread);

describe('ThreadIssuesCard', () => {
    it('is absent when the thread touched no issue, and never lists a PR', () => {
        const { rerender } = render(<ThreadIssuesCard thread={thread([link({})])} />);
        expect(screen.queryByRole('region', { name: 'links.issues' })).not.toBeInTheDocument();
        rerender(<ThreadIssuesCard thread={thread([link({}), issue()])} />);
        const card = screen.getByRole('region', { name: 'links.issues' });
        expect(within(card).getByRole('link', { name: '#3 Flaky test' })).toBeInTheDocument();
        expect(within(card).queryByRole('link', { name: '#12 Add the tool' })).not.toBeInTheDocument();
    });
});

describe('ThreadPrsCard', () => {
    it('is absent when the thread has no PR and nothing to open one with', () => {
        render(<ThreadPrsCard thread={thread([issue()])} />);
        expect(screen.queryByRole('region', { name: 'links.prs' })).not.toBeInTheDocument();
    });

    it('lists each PR with its link, where it stands and its checks, and no issue', () => {
        render(<ThreadPrsCard thread={thread([
            link({ ci: 'failing', merge_state: 'conflicts' }),
            issue(),
        ])} />);
        const card = screen.getByRole('region', { name: 'links.prs' });
        const pr = within(card).getByRole('link', { name: '#12 Add the tool' });
        expect(pr).toHaveAttribute('href', 'https://github.com/acme/w/pull/12');
        expect(pr).toHaveAttribute('target', '_blank');
        expect(pr).toHaveAttribute('rel', expect.stringContaining('noopener'));
        expect(within(card).getByText('links.checksFailing')).toBeInTheDocument();
        expect(within(card).getByText('links.conflicts')).toBeInTheDocument();
        expect(within(card).queryByRole('link', { name: '#3 Flaky test' })).not.toBeInTheDocument();
    });

    it('shows a merged PR as merged without checks, and a draft as a draft', () => {
        render(<ThreadPrsCard thread={thread([
            link({ state: 'merged', ci: 'failing' }),
            link({ number: 13, title: 'WIP', draft: true }),
        ])} />);
        expect(screen.getByText('links.merged')).toBeInTheDocument();
        expect(screen.queryByText('links.checksFailing')).not.toBeInTheDocument();
        expect(screen.getByText('links.draft')).toBeInTheDocument();
    });

    it('leaves out what GitHub was not asked about yet, and says when a refresh failed', () => {
        render(<ThreadPrsCard thread={thread([link({}), link({ number: 14, title: 'Old', last_error: 'HTTP 403' })])} />);
        expect(screen.queryByText('links.checksPassing')).not.toBeInTheDocument();
        expect(screen.getByText(/links\.stale/)).toBeInTheDocument();
    });

    it('never makes a link of an address that is not https', () => {
        render(<ThreadPrsCard thread={thread([link({ url: 'javascript:alert(1)' })])} />);
        expect(screen.getByText('#12 Add the tool').closest('a')).not.toHaveAttribute('href');
    });

    it('opens an open PR\'s own changes, and only an open one that GitHub has been read about', async () => {
        m.get.mockResolvedValue({ number: 12, base: 'main', head: 'tb/fix', source: 'local', files: [], additions: 0, deletions: 0 });
        render(<ThreadPrsCard thread={thread([
            link({}),
            link({ number: 13, title: 'Done', state: 'merged' }),
            link({ number: 14, title: 'Unread', head: '', base: '' }),
        ])} />);
        expect(screen.getAllByRole('button', { name: /links\.filesOf/ })).toHaveLength(1);
        fireEvent.click(screen.getByRole('button', { name: 'links.filesOf' }));
        await waitFor(() => expect(m.get).toHaveBeenCalledWith('/api/v1/threads/{id}/prs/{number}/diff', { params: { path: { id: 't1', number: 12 } } }));
        expect(await screen.findByRole('dialog')).toHaveTextContent('#12 Add the tool');
        // The PR of the thread's own branch is reviewed, then merged, from the same dialog.
        expect(screen.getByRole('button', { name: 'thread.mergeIt' })).toBeInTheDocument();
    });

    it('says in the dialog where the files came from and what is not in the PR', async () => {
        m.get.mockResolvedValue({ number: 12, base: 'main', head: 'tb/fix', source: 'github', unpushed: 2, base_stale: true, files: [], additions: 0, deletions: 0 });
        render(<ThreadPrsCard thread={thread([link({})])} />);
        fireEvent.click(screen.getByRole('button', { name: 'links.filesOf' }));
        expect(await screen.findByText('thread.prDiff.fromGitHub')).toBeInTheDocument();
        expect(screen.getByText('thread.prDiff.unpushed:2')).toBeInTheDocument();
        // The base is only stale for a diff read here; GitHub's own list has no such thing.
        expect(screen.queryByText('thread.prDiff.baseStale')).not.toBeInTheDocument();
    });

    it('offers to merge a branch with commits and no open PR, in place of the PR it does not have', () => {
        render(<ThreadPrsCard thread={thread([], { commits_ahead: 2, files_changed: 3 })} />);
        const card = screen.getByRole('region', { name: 'links.prs' });
        expect(within(card).getByText(/links\.noPr/)).toBeInTheDocument();
        expect(within(card).getByRole('button', { name: 'thread.mergeIt' })).toBeInTheDocument();
        cleanup();
        // With the branch's PR open, the merge is in its changes, not a second way in.
        render(<ThreadPrsCard thread={thread([link({})], { commits_ahead: 2, files_changed: 3 })} />);
        expect(screen.queryByRole('button', { name: 'thread.mergeIt' })).not.toBeInTheDocument();
    });

    it('says a thread without PR links was merged when it was', () => {
        render(<ThreadPrsCard thread={thread([], { merged_at: now, commits_ahead: 2 })} />);
        expect(screen.getByText('thread.mergedInto')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'thread.mergeIt' })).not.toBeInTheDocument();
    });
});
