import { cleanup, fireEvent, render as renderWithHost, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import type { TranscriptSubagents } from '../../hooks/useTranscript';
import { ThreadPanel } from './ThreadPanel';
import { ProjectsHostProvider } from '../../host/ProjectsHost';

const host = { paths: { list: '/', project: () => '/', folder: () => '/' } };
const render = (ui: React.ReactElement) => renderWithHost(ui, {
    wrapper: ({ children }) => <ProjectsHostProvider host={host}>{children}</ProjectsHostProvider>,
});

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
const m = vi.hoisted(() => ({ ops: undefined as Record<string, ReturnType<typeof vi.fn>> | undefined }));
vi.mock('./threadOps', () => ({ threadOps: () => (m.ops!) }));
vi.mock('../ThreadComposer', () => ({ ThreadComposer: () => <div>Composer</div> }));
vi.mock('./DiffDialog', () => ({ DiffDialog: () => null }));
vi.mock('./ComposerStrip', () => ({ ComposerStrip: () => null }));
vi.mock('../Markdown', () => ({ Markdown: ({ text }: { text: string }) => <div>{text}</div>, CodeBlock: () => null }));

beforeEach(() => {
    localStorage.clear();
    m.ops = {
        respond: vi.fn(), stop: vi.fn(), retry: vi.fn(), finish: vi.fn(), reopen: vi.fn(),
        archiveSession: vi.fn().mockResolvedValue({}), restoreSession: vi.fn().mockResolvedValue({}),
        cleanup: vi.fn().mockResolvedValue({ thread_id: 't', worktree_bytes: 1, tmp_bytes: 1, kept: [], can_archive: true, can_delete: false }),
        deleteThread: vi.fn().mockResolvedValue({}),
        setTitle: vi.fn(), setAutoAnswer: vi.fn(), send: vi.fn(), unqueue: vi.fn(), handoff: vi.fn(),
    };
});
afterEach(() => cleanup());

const th = (over: Partial<Thread>) => ({ id: 't', title: 'Work', status: 'idle', task: '', branch: 'tp/x', cwd: '/repo/.claude/worktrees/x', input_tokens: 0, output_tokens: 0, turns: 0, ...over } as Thread);
const props = (thread: Thread, over: Partial<React.ComponentProps<typeof ThreadPanel>> = {}) => ({ projectId: 'p', threadId: 't', thread, events: [], running: false, disabled: false, onBack: () => {}, onRefresh: vi.fn(), ...over });

const subagents: TranscriptSubagents = {
    list: async () => [],
    own: async () => [],
    locate: async () => {},
    steps: () => ({ loaded: false }),
    load: async () => {},
};

/** The menu in reading order; a rule between groups shows as "—". */
const menuRows = async (): Promise<string[]> => {
    fireEvent.click(screen.getByRole('button', { name: 'common.more' }));
    const menu = await screen.findByRole('menu');
    const isRule = (el: Element) => el.getAttribute('role') === 'separator' || el.classList.contains('MuiDivider-root') || !!el.querySelector('.MuiDivider-root');
    return [...menu.children].map((el) => (isRule(el) ? '—' : el.textContent || ''));
};

describe('the thread menu', () => {
    it('puts resolving first, the reading actions next, and what ends the thread last', async () => {
        render(<ThreadPanel {...props(th({ seq: 3 }), { subagents })} />);
        expect(await menuRows()).toEqual([
            'thread.resolve',
            '—',
            'thread.copyRef',
            'thread.showTask',
            'thread.showTrajectory',
            'thread.expandTools',
            'subagents.title',
            '—',
            'session.restart',
            'thread.archive',
        ]);
    });

    it('offers Reopen where Resolve was, and no Restart, for a resolved thread', async () => {
        render(<ThreadPanel {...props(th({ seq: 3, status: 'resolved' }))} />);
        expect(await menuRows()).toEqual([
            'thread.reopen',
            '—',
            'thread.copyRef',
            'thread.showTask',
            'thread.showTrajectory',
            'thread.expandTools',
            '—',
            'thread.archive',
        ]);
    });

    it('keeps an archived thread’s restore and delete last, the delete after the restore', async () => {
        render(<ThreadPanel {...props(th({ seq: 3, status: 'resolved', archived_at: '2026-11-01T00:00:00Z' }))} />);
        expect(await menuRows()).toEqual([
            'thread.copyRef',
            'thread.showTask',
            'thread.showTrajectory',
            'thread.expandTools',
            '—',
            'thread.restore',
            'thread.delete',
        ]);
    });

    it('uses the session labels for a folder thread, resolving before what archives it', async () => {
        render(<ThreadPanel {...props(th({ seq: 3 }), { folder: true })} />);
        expect(await menuRows()).toEqual([
            'session.resolve',
            '—',
            'thread.copyRef',
            'thread.showTask',
            'thread.showTrajectory',
            'thread.expandTools',
            'session.backgroundTasks',
            '—',
            'session.restart',
            'session.handoff',
            'session.archive',
        ]);
    });

    it('no longer shows the run’s model and token usage', async () => {
        render(<ThreadPanel {...props(th({ seq: 3, model: 'cc [1m]', input_tokens: 14_800_000, output_tokens: 58_400 }))} />);
        await menuRows();
        expect(screen.queryByText('cc [1m]')).not.toBeInTheDocument();
        expect(screen.queryByText(/58\.4k out/)).not.toBeInTheDocument();
        // No trailing block after the last action.
        expect(screen.getByRole('menu').lastElementChild).toHaveAttribute('role', 'menuitem');
    });
});
