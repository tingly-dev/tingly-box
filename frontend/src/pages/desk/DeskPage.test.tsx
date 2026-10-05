import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {MemoryRouter} from 'react-router-dom';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import type {ComponentProps} from 'react';
import type SessionView from '@/components/desk/SessionView';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import * as api from '@/services/deskApi';
import DeskPage from './DeskPage';

const {notifyError, t} = vi.hoisted(() => ({
    notifyError: vi.fn(),
    t: (_key: string, opts: {defaultValue: string}) => opts.defaultValue,
}));
vi.mock('react-i18next', () => ({useTranslation: () => ({t})}));
vi.mock('@/hooks/useNotify', () => ({useNotify: () => ({error: notifyError})}));
vi.mock('@/components/desk/useDeskAttention', () => ({requestNotifications: vi.fn(), useDeskAttention: () => new Set()}));
vi.mock('@/services/deskApi', () => ({
    listSessions: vi.fn(), listRecentFolders: vi.fn(), listPermissionModes: vi.fn(), getMessages: vi.fn(), getSession: vi.fn(), sendMessage: vi.fn(), interrupt: vi.fn(), archive: vi.fn(),
}));
vi.mock('@/components/desk/DeskSidebar', () => ({default: ({onSelect}: {onSelect: (id: string) => void}) => (
    <div><button onClick={() => onSelect('a')}>Session A</button><button onClick={() => onSelect('b')}>Session B</button></div>
)}));
vi.mock('@/components/desk/NewSessionView', () => ({default: () => <div>New task form</div>}));
vi.mock('@/components/desk/SessionView', () => ({default: ({session, messages, queued, draft, onSend, onSendQueuedNow, onUnqueue, onInterrupt}: ComponentProps<typeof SessionView>) => (
    <div><span>Viewing {session.id}: {session.status}</span>{messages.map((m, i) => <p key={i}>{m.content}</p>)}
        <p>Draft: {draft}</p>{queued.map((text, i) => <button key={i} onClick={() => onUnqueue(i)}>{text}</button>)}
        <button onClick={() => void onSend('Queued follow-up')}>Add follow-up</button>
        <button onClick={onSendQueuedNow}>Send queue</button>
        <button onClick={() => void onInterrupt()}>Stop turn</button>
    </div>
)}));

const session = (id: string, status = 'completed') => ({id, status, last_activity: '2026-10-03T00:00:00Z'} as SessionInfo);
const message = (content: string) => [{role: 'assistant', content}] as MessageInfo[];
function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((r) => { resolve = r; });
    return {promise, resolve};
}
const open = (path = '/desk?session=a') => render(<MemoryRouter initialEntries={[path]}><DeskPage/></MemoryRouter>);
const settle = () => act(async () => {});

beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.mocked(api.listSessions).mockResolvedValue([session('a'), session('b')]);
    vi.mocked(api.listRecentFolders).mockResolvedValue([]);
    vi.mocked(api.listPermissionModes).mockResolvedValue([]);
    vi.mocked(api.getSession).mockImplementation(async (id) => session(id));
    vi.mocked(api.getMessages).mockImplementation(async (id) => message(`Transcript ${id}`));
    vi.mocked(api.sendMessage).mockResolvedValue(undefined);
    vi.mocked(api.interrupt).mockResolvedValue(undefined);
});
afterEach(() => { cleanup(); sessionStorage.clear(); vi.useRealTimers(); });

describe('Desk local workflow', () => {
    it('never displays another session’s transcript while the new one loads', async () => {
        open();
        await settle();
        expect(screen.getByText('Transcript a')).toBeInTheDocument();
        const next = deferred<MessageInfo[]>();
        vi.mocked(api.getMessages).mockImplementation((id) => id === 'b' ? next.promise : Promise.resolve(message('Transcript a')));
        fireEvent.click(screen.getByRole('button', {name: 'Session B'}));
        await settle();
        expect(screen.getByText('Viewing b: completed')).toBeInTheDocument();
        expect(screen.queryByText('Transcript a')).not.toBeInTheDocument();
        await act(async () => next.resolve(message('Transcript b')));
        expect(screen.getByText('Transcript b')).toBeInTheDocument();
    });

    it('discards an old response even after switching away and back to the same session', async () => {
        const old = deferred<MessageInfo[]>();
        vi.mocked(api.getMessages).mockImplementationOnce(() => old.promise);
        open();
        await settle();
        fireEvent.click(screen.getByRole('button', {name: 'Session B'}));
        await settle();
        fireEvent.click(screen.getByRole('button', {name: 'Session A'}));
        await settle();
        expect(screen.getByText('Transcript a')).toBeInTheDocument();
        await act(async () => old.resolve(message('Outdated transcript')));
        expect(screen.queryByText('Outdated transcript')).not.toBeInTheDocument();
        expect(screen.getByText('Transcript a')).toBeInTheDocument();
    });

    it('retains the transcript on refresh failure and recovers on reconnect without toast spam', async () => {
        open();
        await settle();
        vi.mocked(api.getMessages).mockRejectedValue(new Error('server unavailable'));
        await act(async () => vi.advanceTimersByTimeAsync(10_000));
        expect(screen.getByText('Transcript a')).toBeInTheDocument();
        expect(screen.getByRole('alert')).toHaveTextContent('server unavailable');
        expect(notifyError).not.toHaveBeenCalled();
        vi.mocked(api.getMessages).mockResolvedValue(message('Recovered transcript'));
        await act(async () => window.dispatchEvent(new Event('online')));
        expect(screen.queryByRole('alert')).not.toBeInTheDocument();
        expect(screen.getByText('Recovered transcript')).toBeInTheDocument();
    });

    it('offers retry after initial failure instead of showing an empty task form', async () => {
        vi.mocked(api.listSessions).mockRejectedValue(new Error('offline'));
        open('/desk');
        await settle();
        expect(screen.queryByText('New task form')).not.toBeInTheDocument();
        vi.mocked(api.listSessions).mockResolvedValue([session('a')]);
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Retry'})));
        expect(screen.getByText('New task form')).toBeInTheDocument();
        expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    it('keeps a newer selected-session status when an older list refresh arrives', async () => {
        open();
        await settle();
        const stale = deferred<SessionInfo[]>();
        vi.mocked(api.listSessions).mockImplementationOnce(() => stale.promise);
        vi.mocked(api.getSession).mockResolvedValue(session('a', 'running'));
        await act(async () => window.dispatchEvent(new Event('online')));
        expect(screen.getByText('Viewing a: running')).toBeInTheDocument();
        await act(async () => stale.resolve([session('a'), session('b')]));
        expect(screen.getByText('Viewing a: running')).toBeInTheDocument();
    });

    it('shows an unavailable session explicitly instead of silently opening a new task', async () => {
        open('/desk?session=missing');
        await settle();
        expect(screen.queryByText('New task form')).not.toBeInTheDocument();
        expect(screen.getByRole('alert')).toHaveTextContent('This session is unavailable');
    });

    it('discards an older detail response after a newer list refresh updates the session', async () => {
        open();
        await settle();
        const old = deferred<SessionInfo>();
        vi.mocked(api.getSession).mockImplementationOnce(() => old.promise);
        await act(async () => window.dispatchEvent(new Event('online')));
        vi.mocked(api.listSessions).mockResolvedValue([session('a', 'running'), session('b')]);
        vi.mocked(api.getSession).mockResolvedValue(session('a', 'running'));
        await act(async () => window.dispatchEvent(new Event('online')));
        expect(screen.getByText('Viewing a: running')).toBeInTheDocument();
        await act(async () => old.resolve(session('a')));
        expect(screen.getByText('Viewing a: running')).toBeInTheDocument();
    });
});


describe('Desk queue delivery', () => {
    const saved = () => JSON.parse(sessionStorage.getItem('desk.sessionQueues') ?? '{}');
    it('holds recovered text through polling and keeps the in-flight prefix until acceptance', async () => {
        sessionStorage.setItem('desk.sessionQueues', JSON.stringify({a: {items: ['Recovered message'], inFlight: true}}));
        open();
        await settle();
        await act(async () => {await vi.advanceTimersByTimeAsync(6000);});
        expect(api.sendMessage).not.toHaveBeenCalled();
        const accepted = deferred<void>();
        vi.mocked(api.sendMessage).mockReturnValueOnce(accepted.promise);
        fireEvent.click(screen.getByRole('button', {name: 'Send queue'}));
        fireEvent.click(screen.getByRole('button', {name: 'Send queue'}));
        expect(api.sendMessage).toHaveBeenCalledExactlyOnceWith('a', 'Recovered message');
        expect(saved().a.items).toEqual(['Recovered message']);
        fireEvent.click(screen.getByRole('button', {name: 'Recovered message'}));
        expect(screen.getByRole('button', {name: 'Recovered message'})).toBeInTheDocument();
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Add follow-up'})));
        expect(saved().a.items).toEqual(['Recovered message', 'Queued follow-up']);
        vi.mocked(api.getSession).mockImplementation(async (id) => session(id, 'running'));
        await act(async () => accepted.resolve());
        expect(saved().a.items).toEqual(['Queued follow-up']);
        expect(api.sendMessage).toHaveBeenCalledTimes(1);
    });

    it('does not automatically retry a failed send on later session activity', async () => {
        sessionStorage.setItem('desk.sessionQueues', JSON.stringify({a: {items: ['Keep queued']}}));
        open();
        await settle();
        vi.mocked(api.sendMessage).mockRejectedValueOnce(new Error('Disconnected'));
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send queue'})));
        expect(saved().a).toMatchObject({items: ['Keep queued'], held: 'failed', inFlight: false});
        vi.mocked(api.getSession).mockResolvedValue({...session('a'), last_activity: '2026-10-04T12:00:00Z'});
        await act(async () => {await vi.advanceTimersByTimeAsync(6000);});
        expect(api.sendMessage).toHaveBeenCalledTimes(1);
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send queue'})));
        expect(saved().a.items).toEqual([]);
        expect(api.sendMessage).toHaveBeenCalledTimes(2);
    });

    it('delivers follow-ups once when a running turn completes', async () => {
        vi.mocked(api.listSessions).mockResolvedValue([session('a', 'running')]);
        vi.mocked(api.getSession).mockResolvedValue(session('a', 'running'));
        open();
        await settle();
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Add follow-up'})));
        expect(api.sendMessage).not.toHaveBeenCalled();
        expect(saved().a.items).toEqual(['Queued follow-up']);
        vi.mocked(api.getSession).mockResolvedValue(session('a'));
        await act(async () => {await vi.advanceTimersByTimeAsync(1500);});
        expect(api.sendMessage).toHaveBeenCalledExactlyOnceWith('a', 'Queued follow-up');
        expect(saved().a.items).toEqual([]);
    });

    it('stopping returns unsent follow-ups to the saved draft even if interrupt fails', async () => {
        sessionStorage.setItem('desk.sessionQueues', JSON.stringify({a: {items: ['Keep queued']}}));
        sessionStorage.setItem('desk.sessionDrafts', JSON.stringify({a: 'Existing draft'}));
        open();
        await settle();
        vi.mocked(api.interrupt).mockRejectedValueOnce(new Error('Offline'));
        await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Stop turn'})));
        expect(saved().a.items).toEqual([]);
        expect(JSON.parse(sessionStorage.getItem('desk.sessionDrafts')!).a).toBe('Keep queued\n\nExisting draft');
        expect(api.sendMessage).not.toHaveBeenCalled();
    });
});


it('removes an accepted queue without waiting for a slow transcript refresh', async () => {
    sessionStorage.setItem('desk.sessionQueues', JSON.stringify({a: {items: ['Accepted message']}}));
    open();
    await settle();
    vi.mocked(api.getMessages).mockReturnValue(new Promise(() => {}));
    vi.mocked(api.getSession).mockReturnValue(new Promise(() => {}));
    await act(async () => fireEvent.click(screen.getByRole('button', {name: 'Send queue'})));
    expect(api.sendMessage).toHaveBeenCalledExactlyOnceWith('a', 'Accepted message');
    expect(JSON.parse(sessionStorage.getItem('desk.sessionQueues')!).a.items).toEqual([]);
    expect(screen.getByText('Viewing a: pending')).toBeInTheDocument();
});
