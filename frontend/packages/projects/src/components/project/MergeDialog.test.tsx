import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ApiError } from '../../client';
import { MergeDialog } from './MergeDialog';

const m = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o?.number != null ? `${key}:${o.number}` : key) }) }));
vi.mock('../../client', async (actual) => ({ ...await actual<typeof import('../../client')>(), api: { POST: m.post }, unwrap: (p: Promise<unknown>) => p }));

afterEach(() => {
    cleanup();
    vi.clearAllMocks();
    localStorage.clear();
});

const thread = (o: Partial<Thread> = {}) => ({
    id: 't1', project_id: 'p1', title: 'Cache idempotency keys', last_report: 'Moved lookups to Redis.', branch: 'projects/t1', base_branch: 'main', repo_path: '/code/api', links: [], ...o,
} as Thread);

function open(th: Thread = thread()) {
    const onMerged = vi.fn();
    const view = render(<MergeDialog thread={th} open onClose={vi.fn()} onMerged={onMerged} />);
    return { onMerged, view };
}
const submitName = (n: string) => screen.getByRole('button', { name: n });

describe('MergeDialog', () => {
    it('offers exactly two methods and starts on squash, with the thread title as the commit message', () => {
        open();
        const radios = screen.getAllByRole('radio');
        expect(radios.map((r) => r.textContent)).toEqual([expect.stringContaining('thread.merge.squash'), expect.stringContaining('thread.merge.mr')]);
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
        expect(screen.getByLabelText('thread.merge.message')).toHaveValue('Cache idempotency keys');
        expect(screen.getByText('thread.merge.squashNote')).toBeInTheDocument();
    });

    it('squashes with the edited message and tells the caller', async () => {
        m.post.mockResolvedValue({});
        const { onMerged } = open();
        fireEvent.change(screen.getByLabelText('thread.merge.message'), { target: { value: 'Use Redis for idempotency keys' } });
        fireEvent.click(submitName('thread.merge.doSquash'));
        await waitFor(() => expect(onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'squash', message: 'Use Redis for idempotency keys' } });
    });

    it('opens a merge request with a title and description from the thread, and remembers the choice per project', async () => {
        m.post.mockResolvedValue({});
        const first = open();
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByLabelText('thread.merge.prTitle')).toHaveValue('Cache idempotency keys');
        expect(screen.getByLabelText('thread.merge.prBody')).toHaveValue('Moved lookups to Redis.');
        fireEvent.click(submitName('thread.merge.doOpen'));
        await waitFor(() => expect(first.onMerged).toHaveBeenCalled());
        expect(m.post).toHaveBeenCalledWith('/api/v1/threads/{id}/merge', { params: { path: { id: 't1' } }, body: { method: 'merge_request', title: 'Cache idempotency keys', body: 'Moved lookups to Redis.' } });
        cleanup();

        open();
        expect(screen.getByRole('radio', { name: /thread\.merge\.mr/ })).toHaveAttribute('aria-checked', 'true');
        cleanup();
        open(thread({ project_id: 'another' }));
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
    });

    it('does not remember a method that failed', async () => {
        m.post.mockRejectedValue(new ApiError('gh is missing', 400));
        const { onMerged } = open();
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        fireEvent.click(submitName('thread.merge.doOpen'));
        expect(await screen.findByText('gh is missing')).toBeInTheDocument();
        expect(onMerged).not.toHaveBeenCalled();
        cleanup();
        open();
        expect(screen.getByRole('radio', { name: /thread\.merge\.squash/ })).toHaveAttribute('aria-checked', 'true');
    });

    it('links an open PR of the branch instead of asking for a second one', async () => {
        m.post.mockResolvedValue({});
        const link = { kind: 'pr', repo: 'acme/api', number: 214, state: 'open', head: 'projects/t1', base: 'main' } as never;
        const { onMerged } = open(thread({ links: [link] }));
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByText('thread.merge.mrExisting:214')).toBeInTheDocument();
        expect(screen.queryByLabelText('thread.merge.prTitle')).not.toBeInTheDocument();
        fireEvent.click(submitName('thread.merge.doPush'));
        await waitFor(() => expect(onMerged).toHaveBeenCalled());
    });

    it('ignores a PR of another branch or one that is no longer open', () => {
        const other = { kind: 'pr', number: 1, state: 'open', head: 'other' } as never;
        const merged = { kind: 'pr', number: 2, state: 'merged', head: 'projects/t1' } as never;
        open(thread({ links: [other, merged] }));
        fireEvent.click(screen.getByRole('radio', { name: /thread\.merge\.mr/ }));
        expect(screen.getByLabelText('thread.merge.prTitle')).toBeInTheDocument();
    });

    it('shows what GitHub refused and keeps the dialog open', async () => {
        m.post.mockRejectedValue(new ApiError('PR #11 is open but was not merged, and stays open: Pull Request is not mergeable', 400));
        const { onMerged } = open();
        fireEvent.click(submitName('thread.merge.doSquash'));
        expect(await screen.findByText(/stays open: Pull Request is not mergeable/)).toBeInTheDocument();
        expect(onMerged).not.toHaveBeenCalled();
        expect(submitName('thread.merge.doSquash')).toBeEnabled();
    });

    it('cannot squash with an empty message', () => {
        open();
        fireEvent.change(screen.getByLabelText('thread.merge.message'), { target: { value: '  ' } });
        expect(submitName('thread.merge.doSquash')).toBeDisabled();
    });
});
