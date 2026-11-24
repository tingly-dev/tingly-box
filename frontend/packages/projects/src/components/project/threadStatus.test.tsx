import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { NumberedTitle, stateTone, threadRef, ThreadBadges, ThreadState, ThreadSubtitle } from './threadStatus';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key) }) }));
afterEach(cleanup);

const thread = (o: Partial<Thread> = {}): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'idle', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '',
    ...o,
} as Thread);

describe('ThreadState', () => {
    it('colours only what needs the user and what failed', () => {
        expect(stateTone('waiting')).toBe('attention');
        expect(stateTone('failed')).toBe('failed');
        for (const s of ['working', 'ready', 'idle', 'resolved']) expect(stateTone(s)).toBe('neutral');
    });

    it('shows the word beside the mark, or only the mark when compact', () => {
        const { rerender } = render(<ThreadState status="waiting" />);
        expect(screen.getByText('thread.status.waiting')).toBeTruthy();
        rerender(<ThreadState status="waiting" compact />);
        expect(screen.queryByText('thread.status.waiting')).toBeNull();
        expect(document.querySelector('[data-status="waiting"]')?.getAttribute('title')).toBe('thread.status.waiting');
    });
});

describe('a queued thread', () => {
    it('reads Queued, not Working, and does not spin', () => {
        const { rerender } = render(<ThreadState status="working" queued />);
        expect(screen.getByText('thread.status.queued')).toBeTruthy();
        expect(screen.queryByText('thread.status.working')).toBeNull();
        expect(document.querySelector('[data-status="queued"]')).toBeTruthy();
        expect(document.querySelector('[style*="spin"], [class*="spin"]')).toBeNull();
        rerender(<ThreadState status="working" />);
        expect(screen.getByText('thread.status.working')).toBeTruthy();
    });

    it('ignores a stale queued flag once the thread is no longer working', () => {
        render(<ThreadState status="idle" queued />);
        expect(screen.getByText('thread.status.idle')).toBeTruthy();
    });

    it('says why it waits in the subtitle', () => {
        render(<ThreadSubtitle thread={thread({ status: 'working', queued: true, last_report: 'earlier report' })} />);
        expect(screen.getByText('thread.queuedSubtitle')).toBeTruthy();
    });
});

describe('a thread that runs background tasks', () => {
    it('says so in the subtitle instead of repeating its last report', () => {
        render(<ThreadSubtitle thread={thread({ status: 'working', running_tasks: 2, last_report: 'earlier report' })} />);
        expect(screen.getByText('thread.tasksSubtitle:2')).toBeTruthy();
        expect(screen.queryByText('earlier report')).toBeNull();
    });

    it('keeps what it waits on for the user when it also has tasks', () => {
        render(<ThreadSubtitle thread={thread({ status: 'waiting', wait_reason: 'Which database?', running_tasks: 1 })} />);
        expect(screen.getByText('Which database?')).toBeTruthy();
    });

    it('carries a count pill, with or without commits', () => {
        const { rerender } = render(<ThreadBadges thread={thread({ status: 'working', running_tasks: 3 })} />);
        expect(screen.getByText('thread.tasksBadge:3')).toBeTruthy();
        rerender(<ThreadBadges thread={thread({ status: 'working', running_tasks: 1, branch: 'tb/fix', commits_ahead: 2 })} work />);
        expect(screen.getByText('thread.tasksBadge:1')).toBeTruthy();
        expect(screen.getByText('thread.commits:2')).toBeTruthy();
        rerender(<ThreadBadges thread={thread({ status: 'working', running_tasks: 3 })} tasks={false} />);
        expect(screen.queryByText(/tasksBadge/)).toBeNull();
        rerender(<ThreadBadges thread={thread({ status: 'working', running_tasks: 0 })} />);
        expect(screen.queryByText(/tasksBadge/)).toBeNull();
    });
});

describe('ThreadBadges', () => {
    it('carries the commits as a badge whose tooltip holds the branch and the files, in the header only', () => {
        const th = thread({ branch: 'tb/fix', base_branch: 'main', commits_ahead: 2, files_changed: 3 });
        const { rerender } = render(<ThreadBadges thread={th} links />);
        // A list row or a card says nothing of commits: no count tells the user what to do next.
        expect(screen.queryByText('thread.commits:2')).toBeNull();
        rerender(<ThreadBadges thread={th} work />);
        expect(screen.getByText('thread.commits:2')).toBeTruthy();
        expect(screen.getByTitle('tb/fix · thread.commits:2 · thread.files:3')).toBeTruthy();
    });

    it('says merged, and nothing for a thread without work', () => {
        const merged = thread({ branch: 'tb/fix', base_branch: 'main', merged_at: '2026-01-01T00:00:00Z' });
        // Merged shows in lists and on cards too, unlike the commits.
        const { rerender } = render(<ThreadBadges thread={merged} />);
        expect(screen.getByText('thread.mergedBadge')).toBeTruthy();
        rerender(<ThreadBadges thread={merged} work />);
        expect(screen.getByText('thread.mergedBadge')).toBeTruthy();
        expect(screen.getByTitle('thread.mergedInto')).toBeTruthy();
        rerender(<ThreadBadges thread={thread({ branch: 'tb/fix' })} work />);
        expect(screen.queryByText('thread.commits:0')).toBeNull();
    });

    it('opens what the tasks count counts from the pill, without opening the thread', () => {
        const onTasks = vi.fn();
        const onOpenThread = vi.fn();
        render(<div onClick={onOpenThread}><ThreadBadges thread={thread({ status: 'working', running_tasks: 3 })} onTasks={onTasks} /></div>);
        const pill = screen.getByRole('button', { name: 'thread.tasksSubtitle:3' });
        fireEvent.click(pill);
        expect(onTasks).toHaveBeenCalledTimes(1);
        expect((onTasks.mock.calls[0][0] as Thread[])[0].id).toBe('t');
        expect(onTasks.mock.calls[0][1]).toBeInstanceOf(HTMLElement);
        // The click stops at the pill: the row's own open does not fire.
        expect(onOpenThread).not.toHaveBeenCalled();
    });

    it('stays a plain count without a handler, even while tasks run', () => {
        render(<ThreadBadges thread={thread({ status: 'working', running_tasks: 3 })} />);
        expect(screen.queryByRole('button', { name: 'thread.tasksSubtitle:3' })).toBeNull();
        expect(screen.getByTitle('thread.tasksSubtitle:3')).toBeTruthy();
    });
});

describe('thread numbers', () => {
    it('mentions a thread as "#3 Title", the coordinator by its title', () => {
        expect(threadRef({ seq: 3, title: 'Fix login' })).toBe('#3 Fix login');
        expect(threadRef({ seq: 0, title: 'Payments' })).toBe('Payments');
    });

    it('puts the number before the title, none for the coordinator', () => {
        const { container, rerender } = render(<NumberedTitle thread={{ seq: 3, title: 'Fix login' }} />);
        expect(container.textContent).toBe('#3Fix login');
        expect(container.querySelector('[data-thread-number]')?.textContent).toBe('#3');
        rerender(<NumberedTitle thread={{ seq: 0, title: 'Payments' }} />);
        expect(container.querySelector('[data-thread-number]')).toBeNull();
    });
});
