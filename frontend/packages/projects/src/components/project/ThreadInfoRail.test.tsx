import { cleanup, fireEvent, render as renderBare, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import type { BackgroundTaskRow } from '../workdir/backgroundTasks';
import { readLocal } from '../../utils/storage';
import { ThreadInfoRail } from './ThreadInfoRail';
import type { ReactElement } from 'react';
import { ProjectsHostProvider, type ProjectsHost } from '../../host/ProjectsHost';

const host: ProjectsHost = { paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f' } };
const render = (ui: ReactElement) => renderBare(<ProjectsHostProvider host={host}>{ui}</ProjectsHostProvider>);

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, o?: { count?: number; pct?: number }) => (o?.count != null ? `${key}:${o.count}` : o?.pct != null ? `${key}:${o.pct}` : key), i18n: { language: 'en' } }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const thread = (o: Partial<Thread> = {}): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'idle', origin: 'user', agent: 'claude-code', seq: 1, cwd: '/work/app',
    commits_ahead: 2, files_changed: 3, uncommitted: false, turns: 14, input_tokens: 412000, output_tokens: 38000, cache_read_tokens: 300000, cache_write_tokens: 20000, context_tokens: 150000, context_window: 200000, last_turn_ms: 95000, last_turn_at: new Date().toISOString(), session_id: 'abcd1234-0000', created_at: '', updated_at: '',
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
        // A project thread with no background tasks has no card for them.
        expect(screen.queryByRole('region', { name: 'session.backgroundTasks' })).not.toBeInTheDocument();
    });

    it('shows the background tasks of a project thread too, not only a folder session', () => {
        render(<ThreadInfoRail {...props} thread={thread({ status: 'working' })} folder={false} canReview={false} tasks={[task({}), task({ taskId: 'b', callId: 'c2', status: 'completed', description: 'old build' })]} />);
        const card = screen.getByRole('region', { name: 'session.backgroundTasks' });
        expect(within(card).getByText('pnpm test')).toBeInTheDocument();
        expect(within(card).getByRole('button', { name: 'tasks.stopTask' })).toBeInTheDocument();
        expect(within(card).getByText('session.tasksRunning:1')).toBeInTheDocument();
    });

    it('shows the same status and usage as the coordinator: state, context fill, turns, cache', () => {
        render(<ThreadInfoRail {...props} thread={thread({ status: 'working' })} folder={false} tasks={[]} canReview={false} />);
        const status = screen.getByRole('region', { name: 'rail.status' });
        expect(within(status).getByText('thread.status.working')).toBeInTheDocument();
        expect(within(status).getByText('150k / 200k · 75%')).toBeInTheDocument();
        expect(within(status).getByRole('progressbar', { name: 'rail.context' })).toHaveAttribute('aria-valuenow', '75');
        expect(within(status).getByText('14')).toBeInTheDocument();
        expect(within(status).getByText('1m 35s')).toBeInTheDocument();
        const usage = screen.getByRole('region', { name: 'rail.usage' });
        expect(within(usage).getByText('412k / 38k')).toBeInTheDocument();
        expect(within(usage).getByText('300k / 20k')).toBeInTheDocument();
        expect(within(usage).getByText('73%')).toBeInTheDocument();
        expect(screen.getByText('abcd1234')).toBeInTheDocument();
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
