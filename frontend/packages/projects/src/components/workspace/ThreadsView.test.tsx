import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
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

    it('takes the line under a title from the page', () => {
        show({ subtitle: (s) => (s.wait_reason ? `reason: ${s.wait_reason}` : undefined) });
        expect(screen.getByText('reason: Approve npm test')).toBeInTheDocument();
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

describe('ThreadsView folding its groups', () => {
    const FOLDS = 'tingly-project.overview.folds.v1.p1';
    const threads = [
        th('a', 'Dark mode', 'waiting'), th('b', 'Fix links', 'working'), th('e', 'Tidy docs', 'idle'),
        th('d', 'Shipped', 'resolved'), th('f', 'Old rename', 'resolved', { archived_at: '2026-10-01T00:00:00Z' }),
    ];
    const view = (over: Partial<React.ComponentProps<typeof ThreadsView>> = {}) => (
        <ThreadsView scope="p1" threads={threads} onOpen={vi.fn()} countLabel={(n) => `${n} threads`} empty="none" {...over} />
    );
    const header = (name: string) => screen.getByRole('button', { name });
    const board = () => fireEvent.click(screen.getByRole('button', { name: 'overview.view.board' }));

    it('starts with Resolved and Archived folded and the other groups open, in the list and on the board', () => {
        render(view());
        expect(header('overview.group.waiting 1')).toHaveAttribute('aria-expanded', 'true');
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
        expect(header('overview.group.archived 1')).toHaveAttribute('aria-expanded', 'false');
        expect(screen.getByText('Shipped')).not.toBeVisible();
        expect(screen.getByText('Dark mode')).toBeVisible();
        board();
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
        expect(header('overview.group.working 1')).toHaveAttribute('aria-expanded', 'true');
        expect(screen.getByText('Shipped')).not.toBeVisible();
    });

    it('toggles from the whole header row by click, Enter and Space, with aria-controls on the cards', async () => {
        render(view());
        board();
        const resolved = header('overview.group.resolved 1');
        const cards = document.getElementById(resolved.getAttribute('aria-controls') ?? '');
        expect(cards).toContainElement(screen.getByText('Shipped'));
        fireEvent.click(resolved);
        expect(resolved).toHaveAttribute('aria-expanded', 'true');
        expect(screen.getByText('Shipped')).toBeVisible();
        fireEvent.keyDown(resolved, { key: 'Enter' });
        expect(resolved).toHaveAttribute('aria-expanded', 'false');
        fireEvent.keyDown(resolved, { key: ' ' });
        expect(resolved).toHaveAttribute('aria-expanded', 'true');
        // Any group folds, not only the finished ones, and the count stays on the folded header.
        fireEvent.click(header('overview.group.working 1'));
        expect(header('overview.group.working 1')).toHaveAttribute('aria-expanded', 'false');
        // The fold is a short transition; the cards leave the page when it ends.
        await waitFor(() => expect(screen.getByText('Fix links')).not.toBeVisible());
    });

    it('remembers the choice across a reload, shared by the list and the board, and per scope', () => {
        const first = render(view());
        fireEvent.click(header('overview.group.resolved 1'));
        fireEvent.click(header('overview.group.idle 1'));
        expect(JSON.parse(localStorage.getItem(FOLDS) ?? '{}')).toEqual({ resolved: false, idle: true });
        first.unmount();
        render(view());
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'true');
        expect(header('overview.group.idle 1')).toHaveAttribute('aria-expanded', 'false');
        board();
        expect(header('overview.group.idle 1')).toHaveAttribute('aria-expanded', 'false');
        cleanup();
        render(view({ scope: 'p2' }));
        expect(header('overview.group.idle 1')).toHaveAttribute('aria-expanded', 'true');
    });

    it('lives with a broken stored value', () => {
        localStorage.setItem(FOLDS, '[1,"x"]');
        render(view());
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
        localStorage.setItem(FOLDS, '{"resolved":"yes","idle":true}');
        cleanup();
        render(view());
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
        expect(header('overview.group.idle 1')).toHaveAttribute('aria-expanded', 'false');
    });

    it('keeps a folded group folded while threads arrive, and its count follows', () => {
        const { rerender } = render(view());
        board();
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
        rerender(view({ threads: [...threads, th('g', 'Another shipped', 'resolved'), th('h', 'More work', 'working')] }));
        expect(header('overview.group.resolved 2')).toHaveAttribute('aria-expanded', 'false');
        expect(screen.getByText('Another shipped')).not.toBeVisible();
        expect(header('overview.group.working 2')).toHaveAttribute('aria-expanded', 'true');
        // A group the user folded stays folded too.
        fireEvent.click(header('overview.group.working 2'));
        rerender(view({ threads: [...threads, th('i', 'Third', 'working')] }));
        expect(header('overview.group.working 2')).toHaveAttribute('aria-expanded', 'false');
    });

    it('shows a folded group\'s waiting threads on its heading, in the list and on the board', () => {
        const pinned = [th('a', 'Dark mode', 'waiting'), th('b', 'Needs a look', 'failed'), th('c', 'Fix links', 'working')];
        localStorage.setItem('tingly-project.overview.pinned.p1', JSON.stringify(['a', 'b']));
        localStorage.setItem(FOLDS, JSON.stringify({ pinned: true }));
        render(view({ threads: pinned }));
        board();
        const heading = header('overview.pinned 2 · overview.needsYouIn:2');
        expect(heading).toHaveAttribute('aria-expanded', 'false');
        expect(within(heading).getByTitle('overview.needsYouIn:2')).toBeInTheDocument();
        // Open, the cards say it themselves.
        fireEvent.click(heading);
        expect(header('overview.pinned 2')).toBeInTheDocument();
    });

    it('holds a folded-by-default group open for the thread that is open in it, unless the user folded it', () => {
        render(view({ selectedId: 'd' }));
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'true');
        fireEvent.click(header('overview.group.resolved 1'));
        expect(header('overview.group.resolved 1')).toHaveAttribute('aria-expanded', 'false');
    });

    it('folds and unfolds every group from the toolbar', () => {
        render(view());
        fireEvent.click(screen.getByRole('button', { name: 'overview.collapseAll' }));
        for (const k of ['waiting', 'working', 'idle', 'resolved', 'archived']) {
            expect(header(`overview.group.${k} 1`)).toHaveAttribute('aria-expanded', 'false');
        }
        fireEvent.click(screen.getByRole('button', { name: 'overview.expandAll' }));
        for (const k of ['waiting', 'working', 'idle', 'resolved', 'archived']) {
            expect(header(`overview.group.${k} 1`)).toHaveAttribute('aria-expanded', 'true');
        }
        expect(screen.getByRole('button', { name: 'overview.collapseAll' })).toBeInTheDocument();
    });
});
