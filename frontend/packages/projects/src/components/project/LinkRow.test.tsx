import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ThreadLink } from '../../client';

const { post, writeToClipboard } = vi.hoisted(() => ({ post: vi.fn(), writeToClipboard: vi.fn(() => Promise.resolve()) }));
vi.mock('@tingly/ui', async (importOriginal) => ({ ...(await importOriginal()), writeToClipboard }));
vi.mock('../../client', async (importOriginal) => ({ ...(await importOriginal<typeof import('../../client')>()), api: { POST: post } }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${Object.values(o).join(' ')}` : key), i18n: { language: 'en' } }),
}));

import { LinkRow } from './LinksCard';

const now = new Date().toISOString();
const link = (o: Partial<ThreadLink> = {}): ThreadLink => ({
    kind: 'pr', repo: 'acme/w', number: 12, url: 'https://github.com/acme/w/pull/12', title: 'Add the tool',
    role: 'opened', state: 'open', refreshed_at: now, attempted_at: now, created_at: now, ...o,
} as ThreadLink);

const answer = (...results: Array<{ status: string; error?: string }>) => ({
    data: { results: results.map((r) => ({ repo: 'acme/w', kind: 'pr', number: 12, ...r })) }, response: { ok: true, status: 200, statusText: 'OK' },
});

beforeEach(() => {
    post.mockReset();
    writeToClipboard.mockClear();
});
afterEach(() => {
    cleanup();
    vi.useRealTimers();
});

describe('LinkRow actions', () => {
    it('keeps refresh and details in the tab order without any hover', () => {
        render(<LinkRow link={link()} threadId="t1" />);
        // Hidden by opacity until the row is looked at, never out of the DOM or the accessibility tree.
        const refresh = screen.getByRole('button', { name: 'links.refresh' });
        const details = screen.getByRole('button', { name: /links\.detailsOf/ });
        expect(refresh).toBeEnabled();
        expect(details).toHaveAttribute('aria-haspopup', 'dialog');
        expect(details).toHaveAttribute('aria-expanded', 'false');
        const tabbable = screen.getAllByRole('button').concat(screen.getAllByRole('link'));
        expect(tabbable).toEqual(expect.arrayContaining([refresh, details]));
    });

    it('has no refresh where it is not known whose item it is, but still has details', () => {
        render(<LinkRow link={link()} />);
        expect(screen.queryByRole('button', { name: 'links.refresh' })).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: /links\.detailsOf/ })).toBeInTheDocument();
    });

    it('refreshes just this item through its thread, shows it is busy and says what happened', async () => {
        let finish: (v: ReturnType<typeof answer>) => void = () => undefined;
        post.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
        render(<LinkRow link={link({ kind: 'issue', number: 3 })} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: 'links.refresh' }));

        expect(post).toHaveBeenCalledWith('/api/v1/threads/{id}/links/refresh', { params: { path: { id: 't1' } }, body: { repo: 'acme/w', kind: 'issue', number: 3 } });
        const busy = screen.getByRole('button', { name: 'links.refreshing' });
        expect(busy).toHaveAttribute('aria-busy', 'true');
        expect(busy).toBeDisabled();
        fireEvent.click(busy);
        expect(post).toHaveBeenCalledTimes(1);

        await act(async () => finish(answer({ status: 'refreshed' })));
        expect(screen.getByRole('button', { name: 'links.refresh' })).toBeEnabled();
        expect(screen.getByRole('status')).toHaveTextContent('links.refreshed');
    });

    it('says when the item was just read and when GitHub could not be reached', async () => {
        post.mockResolvedValueOnce(answer({ status: 'recent' }));
        render(<LinkRow link={link()} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: 'links.refresh' }));
        await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('links.refreshRecent'));

        post.mockResolvedValueOnce(answer({ status: 'failed', error: 'GitHub is not available' }));
        fireEvent.click(screen.getByRole('button', { name: 'links.refresh' }));
        await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('links.refreshFailed'));
    });

    it('says when GitHub\'s request budget is nearly used up, without calling it a failure of the item', async () => {
        post.mockResolvedValue(answer({ status: 'limited', error: 'GitHub\'s request budget is nearly used up; reads resume at 15:04' }));
        render(<LinkRow link={link()} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: 'links.refresh' }));
        await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('links.refreshLimited'));
    });

    it('says so when the request itself fails', async () => {
        post.mockResolvedValue({ error: { error: 'this thread has no such issue or PR' }, response: { ok: false, status: 404, statusText: 'Not Found' } });
        render(<LinkRow link={link()} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: 'links.refresh' }));
        await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('links.refreshFailed'));
    });
});

describe('LinkRow details', () => {
    const detailed = link({
        merge_state: 'conflicts', ci: 'failing',
        checks: { passing: 3, failing: 2, pending: 1, more: 1, failed: [{ name: 'lint', url: 'https://github.com/acme/w/runs/1' }, { name: 'e2e', url: 'javascript:alert(1)' }] },
    });

    it('pins the details on the button: title, checks with the failing ones by name, merge state, read time and the actions', async () => {
        render(<LinkRow link={detailed} threadId="t1" />);
        const button = screen.getByRole('button', { name: /links\.detailsOf/ });
        fireEvent.click(button);

        const dialog = await screen.findByRole('dialog', { name: 'Add the tool' });
        expect(button).toHaveAttribute('aria-expanded', 'true');
        expect(within(dialog).getByText(/links\.checkFailing/)).toBeInTheDocument();
        expect(within(dialog).getByRole('link', { name: 'lint' })).toHaveAttribute('href', 'https://github.com/acme/w/runs/1');
        // A check address that is not https is text, never a link.
        expect(within(dialog).queryByRole('link', { name: 'e2e' })).not.toBeInTheDocument();
        expect(within(dialog).getByText('e2e')).toBeInTheDocument();
        expect(within(dialog).getByText(/links\.checksMore/)).toBeInTheDocument();
        expect(within(dialog).getByText('links.merge.conflicts')).toBeInTheDocument();
        expect(within(dialog).getByText('links.read')).toBeInTheDocument();
        const open = within(dialog).getByRole('link', { name: 'links.openOnGitHub' });
        expect(open).toHaveAttribute('href', 'https://github.com/acme/w/pull/12');
        expect(open).toHaveAttribute('rel', expect.stringContaining('noopener'));
    });

    it('names only the failing checks and says nothing of the rest beyond their counts', async () => {
        render(<LinkRow link={link({ checks: { passing: 9, failing: 1, pending: 2, failed: [{ name: 'lint' }] } })} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: /links\.detailsOf/ }));
        const dialog = await screen.findByRole('dialog');
        expect(within(dialog).getByRole('list', { name: 'links.checkFailingList' })).toHaveTextContent('lint');
        expect(within(dialog).getAllByRole('listitem')).toHaveLength(1);
        expect(within(dialog).getByText(/links\.checkPending/)).toBeInTheDocument();
    });

    it('leaves out what an older record does not have instead of showing it as unknown', async () => {
        render(<LinkRow link={link()} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: /links\.detailsOf/ }));
        const dialog = await screen.findByRole('dialog');
        expect(within(dialog).queryByText('links.checks')).not.toBeInTheDocument();
        expect(within(dialog).getByText('links.read')).toBeInTheDocument();
    });

    it('closes on Escape and puts the focus back on the button', async () => {
        render(<LinkRow link={detailed} threadId="t1" />);
        const button = screen.getByRole('button', { name: /links\.detailsOf/ });
        fireEvent.click(button);
        await screen.findByRole('dialog');
        fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' });
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
        expect(button).toHaveFocus();
        expect(button).toHaveAttribute('aria-expanded', 'false');
    });

    it('opens from a focused button, and refreshes and copies from inside', async () => {
        post.mockResolvedValue(answer({ status: 'refreshed' }));
        render(<LinkRow link={detailed} threadId="t1" />);
        const opener = screen.getByRole('button', { name: /links\.detailsOf/ });
        opener.focus();
        expect(opener.tagName).toBe('BUTTON'); // a real button: Enter and Space press it
        fireEvent.click(opener);
        const dialog = await screen.findByRole('dialog');

        fireEvent.click(within(dialog).getByRole('button', { name: 'links.refresh' }));
        expect(post).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(within(dialog).getByRole('status')).toHaveTextContent('links.refreshed'));

        fireEvent.click(within(dialog).getByRole('button', { name: 'links.copyLink' }));
        expect(writeToClipboard).toHaveBeenCalledWith('https://github.com/acme/w/pull/12');
    });

    it('shows why the last refresh failed', async () => {
        render(<LinkRow link={link({ last_error: 'HTTP 403 rate limited' })} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: /links\.detailsOf/ }));
        expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 403 rate limited');
    });

    it('offers no GitHub address that is not https', async () => {
        render(<LinkRow link={link({ url: 'javascript:alert(1)' })} threadId="t1" />);
        fireEvent.click(screen.getByRole('button', { name: /links\.detailsOf/ }));
        const dialog = await screen.findByRole('dialog');
        expect(within(dialog).queryByRole('link', { name: 'links.openOnGitHub' })).not.toBeInTheDocument();
        expect(within(dialog).queryByRole('button', { name: 'links.copyLink' })).not.toBeInTheDocument();
    });
});

describe('LinkRow hover preview', () => {
    it('floats the details up after the mouse rests on the row, and takes them away when it leaves', () => {
        vi.useFakeTimers();
        const { container } = render(<LinkRow link={link()} threadId="t1" />);
        const row = container.firstElementChild as HTMLElement;
        fireEvent.pointerEnter(row, { pointerType: 'mouse' });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        act(() => { vi.advanceTimersByTime(400); });
        expect(screen.getByRole('dialog', { name: 'Add the tool' })).toBeInTheDocument();
        // A preview does not take the focus or mark the button as pressed.
        expect(screen.getByRole('button', { name: /links\.detailsOf/ })).toHaveAttribute('aria-expanded', 'false');

        fireEvent.pointerLeave(row, { pointerType: 'mouse' });
        act(() => { vi.advanceTimersByTime(300); });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('stays while the mouse crosses over to the preview', () => {
        vi.useFakeTimers();
        const { container } = render(<LinkRow link={link()} threadId="t1" />);
        const row = container.firstElementChild as HTMLElement;
        fireEvent.pointerEnter(row, { pointerType: 'mouse' });
        act(() => { vi.advanceTimersByTime(400); });
        fireEvent.pointerLeave(row, { pointerType: 'mouse' });
        act(() => { vi.advanceTimersByTime(100); });
        fireEvent.pointerEnter(screen.getByRole('dialog'), { pointerType: 'mouse' });
        act(() => { vi.advanceTimersByTime(500); });
        expect(screen.getByRole('dialog')).toBeInTheDocument();
    });

    it('never opens on touch: the details button is the way in, and nothing is only on hover', () => {
        vi.useFakeTimers();
        const { container } = render(<LinkRow link={link()} threadId="t1" />);
        const row = container.firstElementChild as HTMLElement;
        fireEvent.pointerEnter(row, { pointerType: 'touch' });
        act(() => { vi.advanceTimersByTime(1000); });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'links.refresh' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /links\.detailsOf/ })).toBeInTheDocument();
    });

    it('closes the preview on Escape', () => {
        vi.useFakeTimers();
        const { container } = render(<LinkRow link={link()} threadId="t1" />);
        fireEvent.pointerEnter(container.firstElementChild as HTMLElement, { pointerType: 'mouse' });
        act(() => { vi.advanceTimersByTime(400); });
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        fireEvent.keyDown(document, { key: 'Escape' });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });
});
