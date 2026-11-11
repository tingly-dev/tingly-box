import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { StatusThreadsPopover } from './StatusThreadsPopover';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));

afterEach(cleanup);

const thread = (o: Partial<Thread> = {}): Thread => ({
    id: 't1', title: 'Retry budget in the SDK', status: 'working', task: '', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '2026-11-11T00:00:00Z',
    ...o,
} as Thread);

const props = { label: 'thread.status.working', threads: [thread()], onOpen: vi.fn(), onClose: vi.fn() };

describe('StatusThreadsPopover', () => {
    it('lists the threads of a state and opens one from its row', () => {
        const onOpen = vi.fn();
        render(<StatusThreadsPopover {...props} threads={[thread(), { ...thread(), id: 't2', title: 'Audit logging' }]} onOpen={onOpen} />);
        expect(screen.getByText('Retry budget in the SDK')).toBeInTheDocument();
        expect(screen.getByText('Audit logging')).toBeInTheDocument();
        expect(screen.getByText(/thread\.status\.working · 2/)).toBeInTheDocument();
        fireEvent.click(screen.getByText('Audit logging'));
        expect(onOpen).toHaveBeenCalledWith('t2');
        expect(props.onClose).toHaveBeenCalled();
    });

    it('keeps a working row\'s tasks pill drilling into what it runs', () => {
        const onTasks = vi.fn();
        render(<StatusThreadsPopover {...props} threads={[thread({ running_tasks: 2 })]} onTasks={onTasks} />);
        fireEvent.click(screen.getByRole('button', { name: 'thread.tasksSubtitle:2' }));
        expect(onTasks).toHaveBeenCalledTimes(1);
        expect((onTasks.mock.calls[0][0] as Thread[])[0].id).toBe('t1');
    });

    it('says when a state has nothing left in it', () => {
        render(<StatusThreadsPopover {...props} threads={[]} />);
        expect(screen.getByText('overview.noThreads')).toBeInTheDocument();
    });
});
