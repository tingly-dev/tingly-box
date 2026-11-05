import { cleanup, render, screen, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Project, Thread } from '../../client';
import { CoordinatorStatus } from './CoordinatorStatus';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const project = { id: 'p', name: 'Pay', coordinator_model: 'sonnet', coordinator_effort: 'high' } as Project;
const worker = (status: string): Thread => ({ id: status, status } as Thread);
const coordinator = (o: Partial<Thread> = {}): Thread => ({
    id: 'main', status: 'idle', turns: 4, input_tokens: 90000, output_tokens: 6000, cache_read_tokens: 72000, cache_write_tokens: 3000,
    context_tokens: 57500, context_window: 200000, last_turn_ms: 4200, last_turn_at: new Date().toISOString(), reported_model: 'sonnet-4', session_id: '1c9d5e7a-0000-4000-8000-000000000000', ...o,
} as Thread);
const props = { project, queued: 0, threads: [] as Thread[] };

describe('CoordinatorStatus', () => {
    it('reads the coordinator through the same Status card a thread has', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ status: 'working' })} queued={2} />);
        const status = screen.getByRole('region', { name: 'rail.status' });
        expect(within(status).getByText('thread.status.working')).toBeInTheDocument();
        expect(within(status).getByText('57.5k / 200k · 29%')).toBeInTheDocument();
        expect(within(status).getByRole('progressbar', { name: 'rail.context' })).toHaveAttribute('aria-valuenow', '29');
        expect(within(status).getByText('4')).toBeInTheDocument();
        expect(within(status).getByText('4.2s')).toBeInTheDocument();
        expect(within(status).getByText('2')).toBeInTheDocument();
    });

    it('shows a recovery decision as waiting on you, and hides an empty queue', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ status: 'waiting' })} />);
        expect(screen.getByText('thread.status.waiting')).toBeInTheDocument();
        expect(screen.queryByText('rail.queued')).not.toBeInTheDocument();
    });

    it('says how many background tasks run, and in how many threads', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator()} threads={[{ ...worker('working'), id: 'w1', running_tasks: 2 } as Thread, { ...worker('working'), id: 'w2', running_tasks: 1 } as Thread, worker('ready')]} />);
        const threads = screen.getByRole('region', { name: 'rail.threads' });
        expect(within(threads).getByText('rail.backgroundTasks').nextSibling).toHaveTextContent('rail.tasksInThreads');
    });

    it('stays quiet about background tasks when none run', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator()} threads={[worker('working')]} />);
        expect(screen.queryByText('rail.backgroundTasks')).not.toBeInTheDocument();
    });

    it('counts threads by what they need, and carries usage with the cache hit rate', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator()} threads={[worker('working'), worker('working'), worker('waiting'), worker('failed'), worker('ready')]} />);
        const threads = screen.getByRole('region', { name: 'rail.threads' });
        expect(within(threads).getByText('thread.status.working').nextSibling).toHaveTextContent('2');
        expect(within(threads).getByText('thread.status.waiting').nextSibling).toHaveTextContent('2');
        expect(within(threads).getByText('thread.status.ready').nextSibling).toHaveTextContent('1');
        const usage = screen.getByRole('region', { name: 'rail.usage' });
        expect(within(usage).getByText('90k / 6k')).toBeInTheDocument();
        expect(within(usage).getByText('72k / 3k')).toBeInTheDocument();
        expect(within(usage).getByText('80%')).toBeInTheDocument();
    });

    it('still renders before the first turn', () => {
        render(<CoordinatorStatus {...props} coordinator={coordinator({ turns: 0, input_tokens: 0, output_tokens: 0, context_tokens: 0, last_turn_ms: 0, last_turn_at: undefined, reported_model: undefined })} />);
        expect(screen.getByText('thread.status.idle')).toBeInTheDocument();
        expect(screen.getByText('rail.contextEmpty')).toBeInTheDocument();
        expect(screen.queryByRole('region', { name: 'rail.usage' })).not.toBeInTheDocument();
    });
});
