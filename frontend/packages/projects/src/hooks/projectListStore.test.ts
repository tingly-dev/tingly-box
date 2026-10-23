import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PROJECTS_CHANGED } from '../host/ProjectsHost';
import { getProjectList, POLL_MS, RELOAD_DEBOUNCE_MS, reloadProjectListSoon, subscribeProjectList } from './projectListStore';
import type { ProjectEvent } from './useProjectEvents';

const m = vi.hoisted(() => ({ get: vi.fn(), open: vi.fn(), close: vi.fn(), emit: undefined as undefined | ((ev: ProjectEvent) => void) }));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get } }));
vi.mock('./useProjectEvents', () => ({
    openEventStream: (url: string, onEvent: (ev: ProjectEvent) => void) => {
        m.open(url);
        m.emit = onEvent;
        return m.close;
    },
}));

const thread = (): ProjectEvent => ({ type: 'thread', project_id: 'f1', data: {} });

beforeEach(() => {
    vi.useFakeTimers();
    m.get.mockResolvedValue({ data: { projects: [] }, response: new Response() });
});
afterEach(() => {
    vi.useRealTimers();
    for (const f of [m.get, m.open, m.close]) f.mockReset();
});

describe('shared project list', () => {
    it('opens one stream and one request for any number of subscribers, and closes with the last', () => {
        const leave = [subscribeProjectList(() => undefined), subscribeProjectList(() => undefined), subscribeProjectList(() => undefined)];
        expect(m.open).toHaveBeenCalledTimes(1);
        expect(m.open).toHaveBeenCalledWith('/api/v1/projects/events');
        expect(m.get).toHaveBeenCalledTimes(1);
        leave[0]();
        leave[1]();
        expect(m.close).not.toHaveBeenCalled();
        leave[2]();
        expect(m.close).toHaveBeenCalledTimes(1);
    });

    it('turns a burst of events and page reports into one fetch per debounce window', async () => {
        const leave = subscribeProjectList(() => undefined);
        await vi.advanceTimersByTimeAsync(0);
        m.get.mockClear();
        for (let i = 0; i < 4; i++) {
            m.emit?.(thread());
            window.dispatchEvent(new Event(PROJECTS_CHANGED));
            reloadProjectListSoon();
        }
        await vi.advanceTimersByTimeAsync(RELOAD_DEBOUNCE_MS - 1);
        expect(m.get).not.toHaveBeenCalled();
        await vi.advanceTimersByTimeAsync(1);
        expect(m.get).toHaveBeenCalledTimes(1);
        leave();
    });

    it('hands every event to the subscribers that asked for them', () => {
        const a = vi.fn();
        const b = vi.fn();
        const leaveA = subscribeProjectList(() => undefined, a);
        const leaveB = subscribeProjectList(() => undefined, b);
        const ev = { type: 'thread_event', project_id: 'f1', data: {} } as ProjectEvent;
        m.emit?.(ev);
        expect(a).toHaveBeenCalledWith(ev);
        expect(b).toHaveBeenCalledWith(ev);
        leaveA();
        leaveB();
    });
    const counts = (waiting: number) => ({ working: 0, waiting, ready: 0, idle: 0, resolved: 0 });
    const row = (id: string, waiting = 0) => ({ id, type: 'folder', name: id, threads: counts(waiting) });
    const listed = async (...rows: ReturnType<typeof row>[]) => {
        m.get.mockResolvedValue({ data: { projects: rows }, response: new Response() });
        const onChange = vi.fn();
        const leave = subscribeProjectList(onChange);
        await vi.advanceTimersByTimeAsync(0);
        m.get.mockClear();
        onChange.mockClear();
        return { onChange, leave };
    };

    it('updates the row a thread event names from its counts, without fetching', async () => {
        const { onChange, leave } = await listed(row('f1'), row('f2'));
        m.emit?.({ type: 'thread', project_id: 'f1', data: {}, threads: counts(2) });
        expect(onChange).toHaveBeenCalledTimes(1);
        expect(getProjectList()?.find((p) => p.id === 'f1')?.threads.waiting).toBe(2);
        await vi.advanceTimersByTimeAsync(POLL_MS - 1);
        expect(m.get).not.toHaveBeenCalled();
        leave();
    });

    it('applies a project event in place and drops an archived project', async () => {
        const { leave } = await listed(row('f1'), row('f2', 1));
        m.emit?.({ type: 'project', project_id: 'f2', data: { id: 'f2', name: 'renamed' } });
        expect(getProjectList()?.find((p) => p.id === 'f2')).toMatchObject({ name: 'renamed', threads: counts(1) });
        m.emit?.({ type: 'project', project_id: 'f1', data: { id: 'f1', archived_at: '2026-10-21T00:00:00Z' } });
        expect(getProjectList()?.map((p) => p.id)).toEqual(['f2']);
        await vi.advanceTimersByTimeAsync(RELOAD_DEBOUNCE_MS);
        expect(m.get).not.toHaveBeenCalled();
        leave();
    });

    it('fetches for a project it does not have', async () => {
        const { leave } = await listed(row('f1'));
        m.emit?.({ type: 'thread', project_id: 'new', data: {}, threads: counts(1) });
        await vi.advanceTimersByTimeAsync(RELOAD_DEBOUNCE_MS);
        expect(m.get).toHaveBeenCalledTimes(1);
        leave();
    });

    it('keeps an update that arrives while a fetch is on its way', async () => {
        const { leave } = await listed(row('f1'));
        let answer: (v: unknown) => void = () => undefined;
        m.get.mockReturnValue(new Promise((r) => (answer = r)));
        reloadProjectListSoon();
        await vi.advanceTimersByTimeAsync(RELOAD_DEBOUNCE_MS);
        m.emit?.({ type: 'thread', project_id: 'f1', data: {}, threads: counts(3) });
        answer({ data: { projects: [row('f1'), row('f2')] }, response: new Response() });
        await vi.advanceTimersByTimeAsync(0);
        expect(getProjectList()?.map((p) => [p.id, p.threads.waiting])).toEqual([['f1', 3], ['f2', 0]]);
        leave();
    });
});
