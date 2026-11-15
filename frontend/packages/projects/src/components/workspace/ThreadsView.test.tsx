import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ThreadsView } from './ThreadsView';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));
afterEach(() => {
    cleanup();
    localStorage.clear();
});

const th = (id: string, title: string, status: string, over: Partial<Thread> = {}) => ({ id, title, status, updated_at: '2026-10-22T00:00:00Z', ...over }) as Thread;
const sessions = [
    th('a', 'Dark mode', 'waiting', { wait_reason: 'Approve npm test' }),
    th('b', 'Fix links', 'working', { running_tasks: 2 }),
    th('c', 'README intro', 'ready'),
    th('d', 'Old rename', 'resolved', { archived_at: '2026-10-01T00:00:00Z' }),
];
// A working directory's groups: ready folds into idle, archived is its own.
const groups = [{ key: 'waiting', label: 'Waiting' }, { key: 'working', label: 'Working' }, { key: 'idle', label: 'Done' }, { key: 'archived', label: 'Archived', collapsed: true }];
const groupOf = (s: Thread) => (s.archived_at ? 'archived' : s.status === 'ready' ? 'idle' : s.status);
const show = (over: Partial<React.ComponentProps<typeof ThreadsView>> = {}) => render(
    <ThreadsView scope="folder-1" threads={sessions} onOpen={vi.fn()} groups={groups} groupOf={groupOf} countLabel={(n) => `${n} sessions`} empty="none" {...over} />,
);

describe('ThreadsView with a working directory\'s groups', () => {
    it('has the toolbar a project has: the count, the tasks chip and List | Board', () => {
        show();
        expect(screen.getByText('4 sessions')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'thread.tasksBadge:2' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'overview.view.list' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('button', { name: 'overview.view.board' })).toBeInTheDocument();
    });

    it('lists by the groups it was given, folding the collapsed ones', () => {
        show();
        expect(screen.getByRole('region', { name: 'Waiting' })).toBeInTheDocument();
        expect(within(screen.getByRole('region', { name: 'Done' })).getByText('README intro')).toBeInTheDocument();
        // The archived group is folded until opened.
        expect(screen.getByRole('region', { name: 'Archived' })).toBeInTheDocument();
    });

    it('switches to a board of the same groups, with pins, as a project does', () => {
        show();
        fireEvent.click(screen.getByRole('button', { name: 'overview.view.board' }));
        expect(screen.getByRole('group', { name: 'Waiting' })).toBeInTheDocument();
        expect(screen.getByRole('group', { name: 'Done' })).toBeInTheDocument();
        fireEvent.click(within(screen.getByRole('group', { name: 'Waiting' })).getByRole('button', { name: 'overview.pin' }));
        expect(JSON.parse(localStorage.getItem('tingly-project.overview.pinned.folder-1') ?? '[]')).toEqual(['a']);
        expect(screen.getByRole('group', { name: 'overview.pinned' })).toBeInTheDocument();
    });

    it('takes the line under a title from the page, and opens a group from its heading with its label', () => {
        const onGroup = vi.fn();
        show({ subtitle: (s) => (s.wait_reason ? `reason: ${s.wait_reason}` : undefined), onGroup });
        expect(screen.getByText('reason: Approve npm test')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Waiting 1' }));
        expect(onGroup).toHaveBeenCalledWith('waiting', 'Waiting', expect.any(HTMLElement));
    });

    it('opens a session from its row and marks the open one', () => {
        const onOpen = vi.fn();
        show({ onOpen, selectedId: 'b' });
        fireEvent.click(screen.getByText('Fix links'));
        expect(onOpen).toHaveBeenCalledWith('b');
    });

    it('says so when there is nothing, in the list and on the board', () => {
        show({ threads: [] });
        expect(screen.getByText('none')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'overview.view.board' }));
        expect(screen.getByText('none')).toBeInTheDocument();
    });
});
