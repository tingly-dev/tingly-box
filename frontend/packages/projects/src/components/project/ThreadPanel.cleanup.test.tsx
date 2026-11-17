import { cleanup, fireEvent, render as renderWithHost, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
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
    m.ops = {
        respond: vi.fn(), stop: vi.fn(), retry: vi.fn(), finish: vi.fn(), reopen: vi.fn(),
        archiveSession: vi.fn().mockResolvedValue({}), restoreSession: vi.fn().mockResolvedValue({}),
        cleanup: vi.fn().mockResolvedValue({ thread_id: 't', worktree_bytes: 1000, tmp_bytes: 200, kept: ['branch tp/x'], can_archive: true, can_delete: false }),
        deleteThread: vi.fn().mockResolvedValue({}),
        setTitle: vi.fn(), setAutoAnswer: vi.fn(), send: vi.fn(), unqueue: vi.fn(), handoff: vi.fn(),
    };
});
afterEach(() => cleanup());

const th = (over: Partial<Thread>) => ({ id: 't', title: 'Work', status: 'resolved', task: '', branch: 'tp/x', cwd: '/repo/.claude/worktrees/x', input_tokens: 0, output_tokens: 0, turns: 0, ...over } as Thread);
const props = (thread: Thread) => ({ projectId: 'p', threadId: 't', thread, events: [], running: false, disabled: false, onBack: () => {}, onRefresh: vi.fn() });

const menu = async (thread: Thread) => {
    render(<ThreadPanel {...props(thread)} />);
    fireEvent.click(screen.getByRole('button', { name: 'common.more' }));
    return screen.findByText('thread.archive');
};

describe('archiving a project thread', () => {
    it('opens on the cleanup report and archives on confirm', async () => {
        fireEvent.click(await menu(th({ status: 'resolved' })));
        await screen.findByText('thread.archiveConfirm');
        expect(m.ops!.cleanup).toHaveBeenCalled();
        expect(screen.getByText('thread.archiveFrees')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'thread.archive' }));
        await waitFor(() => expect(m.ops!.archiveSession).toHaveBeenCalled());
    });

    it('shows restore and delete for an archived thread', async () => {
        render(<ThreadPanel {...props(th({ status: 'resolved', archived_at: '2026-11-01T00:00:00Z' }))} />);
        fireEvent.click(screen.getByRole('button', { name: 'common.more' }));
        expect(await screen.findByText('thread.restore')).toBeInTheDocument();
        expect(screen.getByText('thread.delete')).toBeInTheDocument();
        fireEvent.click(screen.getByText('thread.restore'));
        await waitFor(() => expect(m.ops!.restoreSession).toHaveBeenCalled());
    });

    it('deletes only with the branch name back when no merged PR proves it merged', async () => {
        render(<ThreadPanel {...props(th({ status: 'resolved', archived_at: '2026-11-01T00:00:00Z' }))} />);
        fireEvent.click(screen.getByRole('button', { name: 'common.more' }));
        fireEvent.click(await screen.findByText('thread.delete'));
        await screen.findByText('thread.deleteConfirm');
        expect(screen.getByText('thread.deleteForce', { exact: false })).toBeInTheDocument();
        // Without the branch typed, confirm refuses and no DELETE went out.
        fireEvent.click(screen.getByRole('button', { name: 'thread.delete' }));
        expect(m.ops!.deleteThread).not.toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('thread.deleteTypeBranchLabel', { exact: false }), { target: { value: 'tp/x' } });
        fireEvent.click(screen.getByRole('button', { name: 'thread.delete' }));
        await waitFor(() => expect(m.ops!.deleteThread).toHaveBeenCalledWith(true, 'tp/x'));
    });
});
