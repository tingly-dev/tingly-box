import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { localSessionsApi, type SessionPage } from '@/services/localSessionsApi';
import LocalSessionsPage from './LocalSessionsPage';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('@/services/localSessionsApi', () => ({ localSessionsApi: { status: vi.fn(), projects: vi.fn(), sessions: vi.fn(), preview: vi.fn() } }));
const entry = (id: string, title: string) => ({ id, title, name: `${id}.jsonl`, size: 1 << 30, modified: '2026-10-31T03:00:00Z' });
const page = (text: string, next_cursor?: string): SessionPage => ({
    events: [{ id: text, type: 'assistant', role: 'assistant', blocks: [{ kind: 'text', text }] }], next_cursor,
    direction: 'backward', file_size: 1 << 30, scanned_bytes: 2 << 20, skipped_records: 0,
});

beforeEach(() => {
    vi.resetAllMocks();
    vi.mocked(localSessionsApi.status).mockResolvedValue({ local_access: true, local_binding: true, enabled: true });
    vi.mocked(localSessionsApi.projects).mockResolvedValue({ entries: [entry('project', 'Project')], total: 1 });
    vi.mocked(localSessionsApi.sessions).mockResolvedValue({ entries: [entry('first', 'First session'), entry('second', 'Second session')], total: 2 });
    vi.mocked(localSessionsApi.preview).mockImplementation(async (_id, cursor) => cursor ? page('Earlier reply') : page('Latest reply', 'older-window'));
});

describe('local session browser', () => {
    it('reads a bounded page and replaces it when loading earlier messages', async () => {
        render(<LocalSessionsPage />);
        expect(await screen.findByText('Latest reply')).toBeInTheDocument();
        expect(localSessionsApi.preview).toHaveBeenCalledTimes(1);
        fireEvent.click(screen.getByRole('button', { name: 'localSessions.earlier' }));
        expect(await screen.findByText('Earlier reply')).toBeInTheDocument();
        expect(screen.queryByText('Latest reply')).not.toBeInTheDocument();
        expect(localSessionsApi.preview).toHaveBeenLastCalledWith('first', 'older-window', 'backward', expect.any(AbortSignal));
    });
    it('jumps back to the latest window even when already browsing backward', async () => {
        render(<LocalSessionsPage />);
        await screen.findByText('Latest reply');
        fireEvent.click(screen.getByRole('button', { name: 'localSessions.earlier' }));
        await screen.findByText('Earlier reply');
        fireEvent.click(screen.getByRole('button', { name: 'localSessions.recent' }));
        expect(await screen.findByText('Latest reply')).toBeInTheDocument();
        expect(localSessionsApi.preview).toHaveBeenLastCalledWith('first', '', 'backward', expect.any(AbortSignal));
    });
    it('refreshes the selected session without silently switching back to the first one', async () => {
        vi.mocked(localSessionsApi.preview).mockImplementation(async id => page(id === 'second' ? 'Second reply' : 'First reply'));
        render(<LocalSessionsPage />);
        await screen.findByText('First reply');
        fireEvent.click(screen.getByText('Second session'));
        await screen.findByText('Second reply');
        fireEvent.click(screen.getByRole('button', { name: 'localSessions.refresh' }));
        await screen.findByText('Second reply');
        expect(localSessionsApi.sessions).toHaveBeenCalledTimes(1);
        expect(localSessionsApi.preview).toHaveBeenLastCalledWith('second', '', 'backward', expect.any(AbortSignal));
    });
    it('does not discover files when the connection is remote', async () => {
        vi.mocked(localSessionsApi.status).mockResolvedValue({ local_access: false, local_binding: true, enabled: true });
        render(<LocalSessionsPage />);
        expect(await screen.findByText('localSessions.localOnly')).toBeInTheDocument();
        expect(localSessionsApi.projects).not.toHaveBeenCalled();
        expect(localSessionsApi.preview).not.toHaveBeenCalled();
    });
    it('requires a loopback-only server binding before discovering files', async () => {
        vi.mocked(localSessionsApi.status).mockResolvedValue({ local_access: false, local_binding: false, enabled: true });
        render(<LocalSessionsPage />);
        expect(await screen.findByText('localSessions.localBinding')).toBeInTheDocument();
        expect(localSessionsApi.projects).not.toHaveBeenCalled();
    });
    it('does not discover files while disabled', async () => {
        vi.mocked(localSessionsApi.status).mockResolvedValue({ local_access: true, local_binding: true, enabled: false });
        render(<LocalSessionsPage />);
        expect(await screen.findByText('localSessions.disabled')).toBeInTheDocument();
        expect(localSessionsApi.projects).not.toHaveBeenCalled();
    });
    it('renders session text literally and keeps tool content collapsed', async () => {
        const payload = '<img src="https://example.com/private" onerror="alert(1)">';
        const data = page(payload);
        data.events[0].blocks.push({ kind: 'tool_use', label: 'Bash', text: 'echo secret' }, { kind: 'attachment', label: 'image' });
        vi.mocked(localSessionsApi.preview).mockResolvedValue(data);
        const { container } = render(<LocalSessionsPage />);
        expect(await screen.findByText(payload)).toBeInTheDocument();
        expect(container.querySelector('img')).toBeNull();
        expect(container.querySelector('script')).toBeNull();
        expect(container.querySelector('details')?.open).toBe(false);
    });
    it('aborts the old preview and ignores a late response after session selection', async () => {
        let finish!: (data: SessionPage) => void;
        let oldSignal: AbortSignal | undefined;
        vi.mocked(localSessionsApi.preview).mockImplementation((id, _cursor, _direction, signal) => {
            if (id === 'second') return Promise.resolve(page('Second reply'));
            oldSignal = signal;
            return new Promise(resolve => { finish = resolve; });
        });
        render(<LocalSessionsPage />);
        await waitFor(() => expect(localSessionsApi.preview).toHaveBeenCalled());
        fireEvent.click(screen.getByText('Second session'));
        expect(await screen.findByText('Second reply')).toBeInTheDocument();
        expect(oldSignal?.aborted).toBe(true);
        await act(async () => { finish(page('Stale reply')); });
        expect(screen.queryByText('Stale reply')).not.toBeInTheDocument();
    });
    it('allows recovery when loading the next session page fails', async () => {
        vi.mocked(localSessionsApi.sessions).mockResolvedValueOnce({ entries: [entry('first', 'First session')], total: 30, next_cursor: 'next-sessions' }).mockRejectedValueOnce(new Error('Listing changed'));
        render(<LocalSessionsPage />);
        await screen.findByText('Latest reply');
        fireEvent.click(screen.getByRole('button', { name: 'localSessions.older' }));
        await screen.findByText(/Listing changed/);
        expect(screen.getByRole('button', { name: 'localSessions.refresh' })).not.toBeDisabled();
    });
});
