import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { MergeDraft, Thread } from '../../client';
import { ApiError } from '../../client';
import { MergeDialog } from './MergeDialog';

const m = vi.hoisted(() => ({ post: vi.fn(), get: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.number != null ? `${key}:${o.number}` : key) }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { POST: m.post, GET: m.get }, unwrap: (p: Promise<unknown>) => p }));

afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    localStorage.clear();
});

const thread = (o: Partial<Thread> = {}) => ({
    id: 't1', project_id: 'p1', title: 'Cache idempotency keys', last_report: 'Moved lookups to Redis.', branch: 'projects/t1', base_branch: 'main', repo_path: '/code/api', links: [], ...o,
} as Thread);
const fromCommits: MergeDraft = { number: 0, source: 'commit', title: 'Use Redis for lookups', body: 'Moved lookups to Redis.' };

async function open(th: Thread = thread(), draft: MergeDraft | Error = fromCommits) {
    if (draft instanceof Error) m.get.mockRejectedValue(draft);
    else m.get.mockResolvedValue(draft);
    const onMerged = vi.fn();
    const view = render(<MergeDialog thread={th} open onClose={vi.fn()} onMerged={onMerged} />);
    await waitFor(() => expect(screen.queryByRole('button', { name: /thread\.merge\.do/ })).toBeEnabled());
    return { onMerged, view };
}
const submitName = (n: string) => screen.getByRole('button', { name: n });

describe('MergeDialog', () => {
    it('offers exactly two methods and starts on squash, worded like gh --fill from the branch\'s commits', async () => {
        await open();
        const radios = screen.getAllByRole('radio');
        expect(radios.map((r) => r.textContent)).toEqual([expect.stringContaining('thread.merge.squash'), expect.stringContaining('thread.merge.mr')]);
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
        expect(m.get).toHaveBeenCalledWith('/api/v1/threads/{id}/merge/draft', { params: { path: { id: 't1' } } });
        // No PR yet: the fields word the PR that is opened, and its words become the commit's.
        expect(screen.getByLabelText('thread.merge.prTitle')).toHaveValue('Use Redis for lookups');
        expect(screen.getByLabelText('thread.merge.prBody')).toHaveValue('Moved lookups to Redis.');
        expect(screen.getByText('thread.merge.squashNote')).toBeInTheDocument();
    });

    it('squashes without a PR by sending the PR\'s words, edited', async () => {
        m.post.mockResolvedValue({});
        const { onMerged } = await open();
        fireEvent.change(screen.getByLabelText('thread.merge.prTitle'), { target: { value: 'Use Redis for idempotency keys' } });
        fireEvent.click(submitName('thread.merge.doSquash'));
        await waitFor(() => expect(onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'squash', title: 'Use Redis for idempotency keys', body: 'Moved lookups to Redis.' } });
    });

    it('squashes an open PR with its own title, number and description, as gh pr merge does', async () => {
        m.post.mockResolvedValue({});
        const link = { kind: 'pr', repo: 'acme/api', number: 214, state: 'open', head: 'projects/t1', base: 'main' } as never;
        const { onMerged } = await open(thread({ links: [link] }), { number: 214, source: 'pr', title: 'Cache idempotency keys', body: 'Edited on GitHub.' });
        expect(screen.getByLabelText('thread.merge.commitTitle')).toHaveValue('Cache idempotency keys (#214)');
        expect(screen.getByLabelText('thread.merge.commitBody')).toHaveValue('Edited on GitHub.');
        fireEvent.click(submitName('thread.merge.doSquash'));
        await waitFor(() => expect(onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'squash', message: 'Cache idempotency keys (#214)\n\nEdited on GitHub.' } });
    });

    it('opens a merge request with the draft\'s title and description, and remembers the choice per project', async () => {
        m.post.mockResolvedValue({});
        const first = await open();
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByLabelText('thread.merge.prTitle')).toHaveValue('Use Redis for lookups');
        fireEvent.click(submitName('thread.merge.doOpen'));
        await waitFor(() => expect(first.onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'merge_request', title: 'Use Redis for lookups', body: 'Moved lookups to Redis.' } });
        cleanup();

        await open();
        expect(screen.getByRole('radio', { name: /thread\.merge\.mr/ })).toHaveAttribute('aria-checked', 'true');
        cleanup();
        await open(thread({ project_id: 'another' }));
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
    });

    it('does not remember a method that failed', async () => {
        m.post.mockRejectedValue(new ApiError('gh is missing', 400));
        const { onMerged } = await open();
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        fireEvent.click(submitName('thread.merge.doOpen'));
        expect(await screen.findByText('gh is missing')).toBeInTheDocument();
        expect(onMerged).not.toHaveBeenCalled();
        cleanup();
        await open();
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
    });

    it('links an open PR of the branch instead of asking for a second one', async () => {
        m.post.mockResolvedValue({});
        const link = { kind: 'pr', repo: 'acme/api', number: 214, state: 'open', head: 'projects/t1', base: 'main' } as never;
        const { onMerged } = await open(thread({ links: [link] }), { number: 214, source: 'pr', title: 'T', body: '' });
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByText('thread.merge.mrExisting:214')).toBeInTheDocument();
        expect(screen.queryByLabelText('thread.merge.prTitle')).not.toBeInTheDocument();
        fireEvent.click(submitName('thread.merge.doPush'));
        await waitFor(() => expect(onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'merge_request' } });
    });

    it('ignores a PR of another branch or one that is no longer open', async () => {
        const other = { kind: 'pr', number: 1, state: 'open', head: 'other' } as never;
        const merged = { kind: 'pr', number: 2, state: 'merged', head: 'projects/t1' } as never;
        await open(thread({ links: [other, merged] }));
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByLabelText('thread.merge.prTitle')).toBeInTheDocument();
    });

    it('shows what GitHub refused and keeps the dialog open', async () => {
        m.post.mockRejectedValue(new ApiError('PR #11 is open but was not merged, and stays open: Pull Request is not mergeable', 400));
        const { onMerged } = await open();
        fireEvent.click(submitName('thread.merge.doSquash'));
        expect(await screen.findByText(/stays open: Pull Request is not mergeable/)).toBeInTheDocument();
        expect(onMerged).not.toHaveBeenCalled();
        expect(submitName('thread.merge.doSquash')).toBeEnabled();
    });

    it('cannot go on with an empty title', async () => {
        await open();
        fireEvent.change(screen.getByLabelText('thread.merge.prTitle'), { target: { value: '  ' } });
        expect(submitName('thread.merge.doSquash')).toBeDisabled();
    });

    it('still merges when the draft cannot be read, starting from the thread\'s title', async () => {
        await open(thread(), new Error('offline'));
        expect(screen.getByLabelText('thread.merge.prTitle')).toHaveValue('Cache idempotency keys');
    });
});
