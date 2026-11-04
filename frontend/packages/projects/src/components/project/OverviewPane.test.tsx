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

describe('OverviewPane view', () => {
    it('starts as the status list and switches to vertical kanban columns that follow the statuses', () => {
        show();
        expect(screen.getByRole('button', { name: 'overview.view.list' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('region', { name: 'overview.group.waiting' })).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'overview.group.waiting' })).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'overview.view.vertical' }));
        expect(localStorage.getItem('tingly-project.overview.view')).toBe('vertical');
        for (const [status, titles] of Object.entries({ waiting: ['Pick a backoff', 'Approve the plan'], failed: ['Broken build'], ready: ['Cache keys'], working: ['SDK retries'], resolved: ['Old tracing'] })) {
            expect(within(lane(status)).getAllByRole('button').map((b) => b.textContent)).toEqual(titles.map((x) => expect.stringContaining(x)));
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
        expect(within(lane('waiting')).getAllByRole('button').map((b) => b.textContent)).toEqual([expect.stringContaining('Pick a backoff'), expect.stringContaining('Approve the plan')]);
        expect(within(lane('resolved')).getByText('Old tracing')).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'overview.group.idle' })).not.toBeInTheDocument();
        // Lanes follow the statuses, what needs the user first.
        expect(screen.getAllByRole('group').map((g) => g.getAttribute('aria-label'))).toEqual(['overview.view.label', 'overview.group.waiting', 'overview.group.failed', 'overview.group.ready', 'overview.group.working', 'overview.group.resolved']);
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
        expect(within(lane('working')).getByRole('button')).toHaveAttribute('aria-current', 'page');
        const other = within(lane('failed')).getByRole('button');
        fireEvent.keyDown(other, { key: 'Enter' });
        expect(onOpen).toHaveBeenLastCalledWith('f1');
        expect(other).not.toHaveAttribute('aria-current');
    });
});
