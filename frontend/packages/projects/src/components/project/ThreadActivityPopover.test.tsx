import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { ThreadActivityPopover } from './ThreadActivityPopover';
import { loadThreadActivity } from './threadActivity';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));
vi.mock('./threadActivity', () => ({ loadThreadActivity: vi.fn() }));

afterEach(cleanup);

const thread = (o: Partial<Thread> = {}): Thread => ({ id: 't1', title: 'Fix login', status: 'working', running_tasks: 1, ...o } as Thread);

const activity = vi.mocked(loadThreadActivity);

const props = {
    targets: [thread()],
    onOpenAgent: vi.fn(),
    onOpenTask: vi.fn(),
    onClose: vi.fn(),
};

describe('ThreadActivityPopover', () => {
    it('lists a thread\'s running tasks and subagents, each row carrying its id', async () => {
        activity.mockResolvedValue({
            tasks: [{ task_id: 'task-1', task_type: 'local_bash', description: 'pnpm test' }, { task_id: 'task-2', task_type: 'local_agent', description: '' }],
            subagents: [{ call: 'call-9', background: true, created_at: '2026-11-11T00:00:00Z', description: 'Explore the repo', event_id: 'e1', status: 'completed', steps: 4, subagent_type: 'Explore' }],
        });
        render(<ThreadActivityPopover {...props} />);
        await waitFor(() => expect(screen.getByText('pnpm test')).toBeInTheDocument());
        expect(screen.getByText(/session\.backgroundTasks/)).toBeInTheDocument();
        expect(screen.getByText(/subagents\.title/)).toBeInTheDocument();
        // The unnamed task falls back to its id; the agent carries its type and its steps.
        expect(screen.getByText('task-2')).toBeInTheDocument();
        expect(screen.getByText(/Explore the repo/)).toBeInTheDocument();
        expect(screen.getByText('Explore')).toBeInTheDocument();
        expect(screen.getByText(/subagents\.steps:4/)).toBeInTheDocument();
        const taskRow = screen.getByText('pnpm test').closest('[data-activity-task]');
        expect(taskRow?.getAttribute('data-activity-thread')).toBe('t1');
    });

    it('opens a task in its thread from its row', async () => {
        activity.mockResolvedValue({ tasks: [{ task_id: 'task-1', task_type: 'local_bash', description: 'pnpm test' }], subagents: [] });
        const onOpenTask = vi.fn();
        render(<ThreadActivityPopover {...props} onOpenTask={onOpenTask} />);
        await waitFor(() => expect(screen.getByText('pnpm test')).toBeInTheDocument());
        fireEvent.click(screen.getByText('pnpm test'));
        expect(onOpenTask).toHaveBeenCalledWith('t1', 'task-1');
        expect(props.onClose).toHaveBeenCalled();
    });

    it('opens a subagent on its own from its row', async () => {
        activity.mockResolvedValue({ tasks: [], subagents: [{ call: 'call-9', background: true, created_at: '', description: 'Explore', event_id: 'e', status: 'running', steps: 2 }] });
        const onOpenAgent = vi.fn();
        render(<ThreadActivityPopover {...props} onOpenAgent={onOpenAgent} />);
        await waitFor(() => expect(screen.getByText('Explore')).toBeInTheDocument());
        fireEvent.click(screen.getByText('Explore'));
        expect(onOpenAgent).toHaveBeenCalledWith('t1', 'call-9');
    });

    it('groups what it lists per thread when several threads are shown', async () => {
        activity.mockImplementation(async (id: string) => ({ tasks: [{ task_id: `task-${id}`, task_type: 'local_bash', description: `job ${id}` }], subagents: [] }));
        const onOpenThread = vi.fn();
        render(<ThreadActivityPopover {...props} targets={[thread(), { ...thread(), id: 't2', title: 'Write docs' }]} onOpenThread={onOpenThread} />);
        await waitFor(() => expect(screen.getByText('job t2')).toBeInTheDocument());
        expect(screen.getByText('Fix login')).toBeInTheDocument();
        expect(screen.getByText('Write docs')).toBeInTheDocument();
        fireEvent.click(screen.getByText('Write docs'));
        expect(onOpenThread).toHaveBeenCalledWith('t2');
    });

    it('says when there is nothing behind the count any more', async () => {
        activity.mockResolvedValue({ tasks: [], subagents: [] });
        render(<ThreadActivityPopover {...props} />);
        await waitFor(() => expect(screen.getByText('tasks.none')).toBeInTheDocument());
        expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });

    it('keeps the thread working when its subagents cannot be read, and shows a failed read', async () => {
        activity.mockRejectedValue(new Error('gone'));
        render(<ThreadActivityPopover {...props} />);
        await waitFor(() => expect(screen.getByText('gone')).toBeInTheDocument());
    });
});
