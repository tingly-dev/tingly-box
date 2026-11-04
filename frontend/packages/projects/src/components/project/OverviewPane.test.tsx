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
    it('starts as the status list and switches to kanban columns that follow the statuses', () => {
        show();
        expect(screen.getByRole('button', { name: 'overview.view.list' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('region', { name: 'overview.group.waiting' })).toBeInTheDocument();
        expect(screen.queryByRole('group', { name: 'overview.group.waiting' })).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'overview.view.kanban' }));
        expect(localStorage.getItem('tingly-project.overview.view')).toBe('kanban');
        for (const [status, titles] of Object.entries({ waiting: ['Pick a backoff', 'Approve the plan'], failed: ['Broken build'], ready: ['Cache keys'], working: ['SDK retries'], resolved: ['Old tracing'] })) {
            expect(within(lane(status)).getAllByRole('button').map((b) => b.textContent)).toEqual(titles.map((x) => expect.stringContaining(x)));
        }
        // A status with no threads has no column.
        expect(screen.queryByRole('group', { name: 'overview.group.idle' })).not.toBeInTheDocument();
        // The wait reason shows on the card, as in the list.
        expect(within(lane('waiting')).getByText('Which one?')).toBeInTheDocument();
    });

    it('remembers the choice and offers it only on the Threads tab', () => {
        localStorage.setItem('tingly-project.overview.view', 'kanban');
        show();
        expect(screen.getByRole('button', { name: 'overview.view.kanban' })).toHaveAttribute('aria-pressed', 'true');
        expect(lane('failed')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('tab', { name: 'overview.library' }));
        expect(screen.queryByRole('group', { name: 'overview.view.label' })).not.toBeInTheDocument();
    });

    it('opens a thread from a card as from a row, and marks the open one', () => {
        localStorage.setItem('tingly-project.overview.view', 'kanban');
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
