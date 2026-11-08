import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { OverviewPane } from './OverviewPane';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));
afterEach(() => {
    cleanup();
    localStorage.clear();
});

const th = (id: string, title: string, status: string, over: Partial<Thread> = {}) => ({ id, title, status, updated_at: '2026-10-22T00:00:00Z', ...over }) as Thread;
const threads = [
    th('w1', 'Pick a backoff', 'waiting', { wait_reason: 'Which one?' }),
    th('w2', 'Approve the plan', 'waiting'),
    th('f1', 'Broken build', 'failed'),
    th('r1', 'Cache keys', 'ready'),
    th('k1', 'SDK retries', 'working'),
    th('d1', 'Old tracing', 'resolved'),
];

const show = (onOpen = vi.fn(), selectedId?: string) => render(<OverviewPane projectId="p" threads={threads} onOpen={onOpen} libraryKey={0} selectedId={selectedId} />);
const lane = (status: string) => screen.getByRole('group', { name: `overview.group.${status}` });
// The card's own button carries no aria-label; the pin star on it does.
const cards = (status: string) => within(lane(status)).getAllByRole('button').filter((b) => b.getAttribute('aria-label') == null);

describe('OverviewPane view', () => {
    it('starts as the status list and switches to vertical kanban columns that follow the statuses', () => {
        show();
        expect(screen.getByRole('button', { name: 'overview.view.list' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('region', { name: 'overview.group.waiting' })).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'overview.group.waiting' })).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'overview.view.vertical' }));
        expect(localStorage.getItem('tingly-project.overview.view')).toBe('vertical');
        for (const [status, titles] of Object.entries({ waiting: ['Pick a backoff', 'Approve the plan'], failed: ['Broken build'], ready: ['Cache keys'], working: ['SDK retries'], resolved: ['Old tracing'] })) {
            expect(cards(status).map((b) => b.textContent)).toEqual(titles.map((x) => expect.stringContaining(x)));
        }
        // A status with no threads has no column.
        expect(screen.queryByRole('group', { name: 'overview.group.idle' })).not.toBeInTheDocument();
        // The wait reason shows on the card, as in the list.
        expect(within(lane('waiting')).getByText('Which one?')).toBeInTheDocument();
    });

    it('horizontal kanban has the same lanes and cards, in rows', () => {
        show();
        fireEvent.click(screen.getByRole('button', { name: 'overview.view.horizontal' }));
        expect(localStorage.getItem('tingly-project.overview.view')).toBe('horizontal');
        expect(screen.getByRole('button', { name: 'overview.view.horizontal' })).toHaveAttribute('aria-pressed', 'true');
        expect(document.querySelector('[data-orientation="horizontal"]')).not.toBeNull();
        expect(cards('waiting').map((b) => b.textContent)).toEqual([expect.stringContaining('Pick a backoff'), expect.stringContaining('Approve the plan')]);
        expect(within(lane('resolved')).getByText('Old tracing')).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'overview.group.idle' })).not.toBeInTheDocument();
        // Lanes follow the statuses: what runs first, then what needs the user, then what waits for review.
        expect(screen.getAllByRole('group').map((g) => g.getAttribute('aria-label'))).toEqual(['overview.view.label', 'overview.group.working', 'overview.group.waiting', 'overview.group.ready', 'overview.group.failed', 'overview.group.resolved']);
    });

    it('never scrolls sideways: the pane clips x and the board is one width-bound grid', () => {
        for (const view of ['vertical', 'horizontal']) {
            localStorage.setItem('tingly-project.overview.view', view);
            const { container, unmount } = show();
            const board = container.querySelector(`[data-orientation="${view}"]`) as HTMLElement;
            expect(getComputedStyle(board).overflowX).toBe('hidden');
            expect(getComputedStyle(board.parentElement as HTMLElement).overflowX).toBe('hidden');
            expect(getComputedStyle(board.parentElement as HTMLElement).overflowY).toBe('auto');
            // Columns wrap to the width (auto-fill), they do not sit in a row that overflows.
            expect(board.style.gridTemplateColumns || getComputedStyle(board).gridTemplateColumns).not.toBe('');
            unmount();
        }
    });

    it('reads the old "kanban" choice as the vertical board', () => {
        localStorage.setItem('tingly-project.overview.view', 'kanban');
        show();
        expect(screen.getByRole('button', { name: 'overview.view.vertical' })).toHaveAttribute('aria-pressed', 'true');
    });

    it('remembers the choice and offers it only on the Threads tab', () => {
        localStorage.setItem('tingly-project.overview.view', 'horizontal');
        show();
        expect(screen.getByRole('button', { name: 'overview.view.horizontal' })).toHaveAttribute('aria-pressed', 'true');
        expect(lane('failed')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('tab', { name: 'overview.library' }));
        expect(screen.queryByRole('group', { name: 'overview.view.label' })).not.toBeInTheDocument();
    });

    it('opens a thread from a card as from a row, and marks the open one', () => {
        localStorage.setItem('tingly-project.overview.view', 'vertical');
        const onOpen = vi.fn();
        show(onOpen, 'k1');
        fireEvent.click(within(lane('working')).getByText('SDK retries'));
        expect(onOpen).toHaveBeenCalledWith('k1');
        expect(cards('working')[0]).toHaveAttribute('aria-current', 'page');
        const other = cards('failed')[0];
        fireEvent.keyDown(other, { key: 'Enter' });
        expect(onOpen).toHaveBeenLastCalledWith('f1');
        expect(other).not.toHaveAttribute('aria-current');
    });

    it('pins cards to the section above every status, newest first, without opening them', () => {
        localStorage.setItem('tingly-project.overview.view', 'vertical');
        const onOpen = vi.fn();
        show(onOpen);
        // The second waiting card's star: pin it.
        fireEvent.click(within(lane('waiting')).getAllByRole('button', { name: 'overview.pin' })[1]);
        expect(onOpen).not.toHaveBeenCalled();
        expect(JSON.parse(localStorage.getItem('tingly-project.overview.pinned.p') ?? '[]')).toEqual(['w2']);
        // It leaves its status for the pinned section, which sits above every group.
        const pinnedLane = () => screen.getByRole('group', { name: 'overview.pinned' });
        expect(within(pinnedLane()).getAllByRole('button').filter((b) => b.getAttribute('aria-label') == null).map((b) => b.textContent)).toEqual([expect.stringContaining('Approve the plan')]);
        expect(screen.getAllByRole('group').map((g) => g.getAttribute('aria-label'))).toEqual(['overview.view.label', 'overview.pinned', 'overview.group.working', 'overview.group.waiting', 'overview.group.ready', 'overview.group.failed', 'overview.group.resolved']);
        // A second pin, from another status, joins above it: newest pin first.
        fireEvent.click(within(lane('failed')).getByRole('button', { name: 'overview.pin' }));
        expect(JSON.parse(localStorage.getItem('tingly-project.overview.pinned.p') ?? '[]')).toEqual(['f1', 'w2']);
        expect(within(pinnedLane()).getAllByRole('button').filter((b) => b.getAttribute('aria-label') == null).map((b) => b.textContent)).toEqual([expect.stringContaining('Broken build'), expect.stringContaining('Approve the plan')]);
        // The status the pinned card left, now empty, disappears; the rest keep their order.
        expect(cards('waiting').map((b) => b.textContent)).toEqual([expect.stringContaining('Pick a backoff')]);
        expect(screen.queryByRole('group', { name: 'overview.group.failed' })).not.toBeInTheDocument();
        // A fresh mount, as a browser reload, reads the stored pins back.
        cleanup();
        show();
        expect(within(pinnedLane()).getByText('Broken build')).toBeInTheDocument();
        expect(within(pinnedLane()).getAllByRole('button', { name: 'overview.unpin' })).toHaveLength(2);
    });

    it('unpins and the cards return to their statuses and order', () => {
        localStorage.setItem('tingly-project.overview.view', 'vertical');
        localStorage.setItem('tingly-project.overview.pinned.p', JSON.stringify(['w2']));
        show();
        const pinnedLane = screen.getByRole('group', { name: 'overview.pinned' });
        expect(within(pinnedLane).getAllByRole('button').filter((b) => b.getAttribute('aria-label') == null).map((b) => b.textContent)).toEqual([expect.stringContaining('Approve the plan')]);
        expect(cards('waiting').map((b) => b.textContent)).toEqual([expect.stringContaining('Pick a backoff')]);
        fireEvent.click(within(pinnedLane).getByRole('button', { name: 'overview.unpin' }));
        expect(JSON.parse(localStorage.getItem('tingly-project.overview.pinned.p') ?? '[]')).toEqual([]);
        expect(screen.queryByRole('group', { name: 'overview.pinned' })).not.toBeInTheDocument();
        expect(cards('waiting').map((b) => b.textContent)).toEqual([expect.stringContaining('Pick a backoff'), expect.stringContaining('Approve the plan')]);
    });

    it('keeps the star quiet: an unpinned card hides it until hover, a pinned one always shows it', () => {
        localStorage.setItem('tingly-project.overview.view', 'vertical');
        show();
        const star = within(lane('working')).getByRole('button', { name: 'overview.pin' });
        expect(getComputedStyle(star).opacity).toBe('0');
        fireEvent.click(star);
        // The pinned card now lives in the pinned section above every status.
        const pinned = screen.getByRole('group', { name: 'overview.pinned' });
        expect(getComputedStyle(within(pinned).getByRole('button', { name: 'overview.unpin' })).opacity).toBe('1');
    });
});

describe('OverviewPane status tab', () => {
    it('offers Status as a third tab beside Threads and Library when the coordinator has a status', () => {
        render(<OverviewPane projectId="p" threads={threads} onOpen={vi.fn()} libraryKey={0} status={<div>coordinator status</div>} />);
        expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual(['overview.threadsWaiting:3', 'overview.library', 'overview.status']);
        expect(screen.queryByText('coordinator status')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('tab', { name: 'overview.status' }));
        expect(screen.getByText('coordinator status')).toBeInTheDocument();
        // The thread list's view switch belongs to the Threads tab only.
        expect(screen.queryByRole('button', { name: 'overview.view.list' })).not.toBeInTheDocument();
    });

    it('opens on the tab asked for, as a window does', () => {
        render(<OverviewPane projectId="p" threads={threads} onOpen={vi.fn()} libraryKey={0} initialTab="status" status={<div>coordinator status</div>} />);
        expect(screen.getByRole('tab', { name: 'overview.status', selected: true })).toBeInTheDocument();
        expect(screen.getByText('coordinator status')).toBeInTheDocument();
    });

    it('has no Status tab without one', () => {
        show();
        expect(screen.queryByRole('tab', { name: 'overview.status' })).not.toBeInTheDocument();
    });
});
