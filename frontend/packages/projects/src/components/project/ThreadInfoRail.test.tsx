import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Subagent, Thread } from '../../client';

const { writeToClipboard } = vi.hoisted(() => ({ writeToClipboard: vi.fn(() => Promise.resolve()) }));
vi.mock('@tingly/ui', async (importOriginal) => ({ ...(await importOriginal()), writeToClipboard }));
import type { BackgroundTaskRow } from '../workdir/backgroundTasks';
import { readLocal } from '../../utils/storage';
import { ThreadInfoRail } from './ThreadInfoRail';
import { keepTail } from './infoRail';

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

    it('keeps a long folder on one line: the ellipsis leads, the tail stays whole, the hover and the copy carry the whole', async () => {
        const cwd = '/Users/me/.tingly-box/projects/p-latency/threads/t-cache';
        render(<ThreadInfoRail {...props} thread={thread({ cwd })} folder={false} tasks={[]} canReview={false} />);
        // The path abbreviates to its trailing directories from a `/`, never wrapping the card tall.
        const shown = screen.getByText('…/threads/t-cache');
        expect(shown.textContent?.startsWith('…')).toBe(true);
        expect(shown.textContent).not.toContain('.tingly-box');
        expect(screen.queryByText('/work/app')).not.toBeInTheDocument();
        // Hovering tells the whole value through the tooltip.
        fireEvent.mouseOver(shown);
        expect(await screen.findByText(cwd)).toBeInTheDocument();
        // A click copies the whole value, and a check says so.
        fireEvent.click(shown);
        await waitFor(() => expect(writeToClipboard).toHaveBeenCalledWith(cwd));
        await waitFor(() => expect(shown.closest('[role="button"]')?.querySelector('svg')).not.toBeNull());
    });

    it('lets a short value show as it always did, with a click that copies it whole', () => {
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false} />);
        const shown = screen.getByText('/work/app');
        expect(shown.textContent).toBe('/work/app');
        fireEvent.click(shown);
        expect(writeToClipboard).toHaveBeenCalledWith('/work/app');
    });

    it('keeps the permissions row on one line too, with a click that copies it', () => {
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false} permission="Project default · Accept edits" />);
        const shown = screen.getByText('Project default · Accept edits');
        expect(shown.textContent).toBe('Project default · Accept edits');
        fireEvent.click(shown);
        expect(writeToClipboard).toHaveBeenCalledWith('Project default · Accept edits');
    });

    it('copies a session id whole while still showing it short', () => {
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false} />);
        fireEvent.click(screen.getByText('abcd1234'));
        expect(writeToClipboard).toHaveBeenCalledWith('abcd1234-0000');
        expect(screen.getByText('abcd1234').textContent).toBe('abcd1234');
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

    it("puts the host's extra cards after Usage", () => {
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false} extra={<section aria-label="host card" />} />);
        const usage = screen.getByRole('region', { name: 'rail.usage' });
        expect(usage.compareDocumentPosition(screen.getByRole('region', { name: 'host card' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    });

    it("lists the thread's subagents in their own card, a row opening the run on its own", async () => {
        const onOpenAgent = vi.fn();
        const onShowAgent = vi.fn();
        const agents: Subagent[] = [
            { call: 'c-early', background: true, created_at: '2026-11-11T01:00:00Z', description: 'First scout', event_id: 'e1', status: 'completed', steps: 3, subagent_type: 'Explore' },
            { call: 'c-late', background: true, created_at: '2026-11-11T02:00:00Z', description: '', event_id: 'e2', status: 'running', steps: 1 },
        ];
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false}
            subagents={{ list: () => Promise.resolve(agents) }} onOpenAgent={onOpenAgent} onShowAgent={onShowAgent} />);
        const card = await waitFor(() => screen.getByRole('region', { name: 'subagents.title' }));
        expect(within(card).getByText('First scout')).toBeInTheDocument();
        // Newest first; an unnamed run falls back to the card's title.
        expect(within(card).getAllByText('subagents.title').length).toBeGreaterThan(1);
        expect(within(card).getByText('Explore')).toBeInTheDocument();
        fireEvent.click(within(card).getByText('First scout'));
        expect(onOpenAgent).toHaveBeenCalledWith(agents[0]);
        // The rows are newest first, so the first scout's Show button is the second one.
        fireEvent.click(within(card).getAllByRole('button', { name: 'subagents.show' })[1]);
        expect(onShowAgent).toHaveBeenCalledWith(agents[0]);
    });

    it('carries no subagent card for a thread that never ran one', async () => {
        render(<ThreadInfoRail {...props} thread={thread()} folder={false} tasks={[]} canReview={false}
            subagents={{ list: () => Promise.resolve([]) }} onOpenAgent={() => undefined} onShowAgent={() => undefined} />);
        expect(screen.getByText('subagents.loading')).toBeInTheDocument();
        await waitFor(() => expect(screen.queryByText('subagents.loading')).not.toBeInTheDocument());
        expect(screen.queryByRole('region', { name: 'subagents.title' })).not.toBeInTheDocument();
    });
});

describe('keepTail', () => {
    const path = '/home/x/tingly-box/.claude/worktrees/stop-28c4c8';

    it('keeps the last two path directories when they fit the budget', () => {
        expect(keepTail(path, 24)).toBe('…/worktrees/stop-28c4c8');
    });

    it('drops earlier directories before ever cutting the tail', () => {
        expect(keepTail(path, 14)).toBe('…/stop-28c4c8');
    });

    it('clips a too-long last segment at its head, never at its end', () => {
        const shown = keepTail(path, 9);
        expect(shown.startsWith('…')).toBe(true);
        expect(shown.endsWith('28c4c8')).toBe(true);
        expect(shown).toHaveLength(9);
    });

    it('lets a value that fits through untouched', () => {
        expect(keepTail('/work/app', 40)).toBe('/work/app');
    });

    it('cuts a prefixed value after a nearby separator so the tail reads whole', () => {
        expect(keepTail('Project default · Accept edits', 18)).toBe('…· Accept edits');
        expect(keepTail('main → tp/stop-28c4c8', 18)).toBe('…→ tp/stop-28c4c8');
    });

    it('falls back to a plain head ellipsis for one long word', () => {
        const shown = keepTail('claude-sonnet-4-5-20250929', 12);
        expect(shown.startsWith('…')).toBe(true);
        expect(shown.endsWith('20250929')).toBe(true);
        expect(shown).toHaveLength(12);
    });
});
