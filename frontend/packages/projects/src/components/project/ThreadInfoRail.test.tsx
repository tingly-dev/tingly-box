import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import type { BackgroundTaskRow } from '../workdir/backgroundTasks';
import { readLocal } from '../../utils/storage';
import { ThreadInfoRail } from './ThreadInfoRail';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number }) => (o?.count != null ? `${key}:${o.count}` : key), i18n: { language: 'en' } }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const thread = (o: Partial<Thread> = {}): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'idle', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 2, files_changed: 3, uncommitted: false, turns: 14, input_tokens: 412000, output_tokens: 38000, created_at: '', updated_at: '',
    branch: 'tb/fix', base_branch: 'main', model: 'sonnet', ...o,
} as Thread);

const task = (o: Partial<BackgroundTaskRow>): BackgroundTaskRow => ({ taskId: 'a', callId: 'c1', status: 'running', background: true, taskType: 'local_bash', description: 'pnpm test', ended: false, ...o } as BackgroundTaskRow);

const props = { onReview: () => undefined, onChanged: () => undefined, onReveal: () => undefined };

describe('ThreadInfoRail', () => {
    it('shows where the thread runs and what it changed, with Review when there is something to review', () => {
        const onReview = vi.fn();
        render(<ThreadInfoRail {...props} onReview={onReview} thread={thread()} folder={false} tasks={[]} canReview />);
        expect(screen.getByText('/work/app')).toBeInTheDocument();
        expect(screen.getByText('main → tb/fix')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'thread.review' }));
        expect(onReview).toHaveBeenCalled();
        // Background tasks belong to a folder session only.
        expect(screen.queryByRole('region', { name: 'session.backgroundTasks' })).not.toBeInTheDocument();
    });

    it('lists running tasks with a stop button, finished ones folded behind a count', () => {
        const onReveal = vi.fn();
        render(<ThreadInfoRail {...props} onReveal={onReveal} thread={thread()} folder canReview={false} tasks={[task({}), task({ taskId: 'b', callId: 'c2', status: 'completed', description: 'old build' })]} />);
        expect(screen.getByRole('button', { name: 'tasks.stopTask' })).toBeInTheDocument();
        fireEvent.click(screen.getByText('pnpm test'));
        expect(onReveal).toHaveBeenCalledWith('c1');
        expect(screen.queryByText('old build')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /rail.finished:1/ }));
        expect(screen.getByText('old build')).toBeInTheDocument();
    });

    it('remembers a folded card', () => {
        const { unmount } = render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false} />);
        fireEvent.click(screen.getByRole('button', { name: /rail.environment/ }));
        expect(readLocal('thread.rail.env')).toBe('0');
        unmount();
    });
});
