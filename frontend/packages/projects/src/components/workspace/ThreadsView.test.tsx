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

describe('ThreadsView with the default project groups', () => {
    it('files an archived thread under its own collapsed group', () => {
        render(
            <ThreadsView scope="p1" threads={sessions} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" />,
        );
        // The archived group holds the thread, folded until opened.
        expect(screen.getByRole('region', { name: 'overview.group.archived' })).toBeInTheDocument();
        expect(screen.getByText('Old rename')).not.toBeVisible();
    });

    it('gives the board an archived section, apart from the statuses', () => {
        render(
            <ThreadsView scope="p1" threads={sessions} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" />,
        );
        fireEvent.click(screen.getByRole('button', { name: 'overview.view.board' }));
        const archived = screen.getByRole('group', { name: 'overview.group.archived' });
        expect(within(archived).getByText('Old rename')).toBeInTheDocument();
        expect(within(screen.getByRole('group', { name: 'overview.group.waiting' })).queryByText('Old rename')).not.toBeInTheDocument();
    });
});

describe('ThreadsView with the select mode', () => {
    it('offers selecting only when the page can bulk-act, and picks rows by click', () => {
        const bulk = { resolve: vi.fn(), archive: vi.fn() };
        const threads = [th('a', 'Dark mode', 'waiting'), th('b', 'Fix links', 'working'), th('c', 'Old', 'resolved', { archived_at: '2026-10-01T00:00:00Z' })];
        render(<ThreadsView scope="p1" threads={threads} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" bulk={bulk} />);
        // The select mode is off: no checkboxes, opens work as before.
        expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'overview.select' }));
        // Archived threads are not pickable.
        expect(screen.getAllByRole('checkbox')).toHaveLength(2);
        fireEvent.click(screen.getByText('Dark mode'));
        fireEvent.click(screen.getByText('Fix links'));
        expect(screen.getByText('overview.selectedCount:2')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'overview.bulkArchive' }));
        expect(bulk.archive).toHaveBeenCalledWith(['a', 'b']);
        expect(bulk.resolve).not.toHaveBeenCalled();
        // The action leaves the select mode.
        expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    });

    it('resolves the picked threads and never counts the archived ones', () => {
        const bulk = { resolve: vi.fn(), archive: vi.fn() };
        const threads = [th('a', 'Dark mode', 'waiting'), th('c', 'Old', 'resolved', { archived_at: '2026-10-01T00:00:00Z' })];
        render(<ThreadsView scope="p1" threads={threads} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" bulk={bulk} />);
        fireEvent.click(screen.getByRole('button', { name: 'overview.select' }));
        fireEvent.click(screen.getByRole('button', { name: 'overview.selectAll' }));
        expect(screen.getByText('overview.selectedCount:1')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'overview.bulkResolve' }));
        expect(bulk.resolve).toHaveBeenCalledWith(['a']);
    });

    it('keeps the toolbar without the select mode when the page cannot bulk-act', () => {
        render(<ThreadsView scope="p1" threads={[th('a', 'Dark mode', 'waiting')]} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" />);
        expect(screen.queryByRole('button', { name: 'overview.select' })).not.toBeInTheDocument();
    });
});
