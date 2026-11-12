import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ProjectSummary, Thread, ThreadEvent } from '../client';

// The store, faked: the engine joins it like a page would, and reads the
// project list to name projects and build links.
const subscribers = new Set<(ev: unknown) => void>();
let rows: ProjectSummary[] = [];
vi.mock('../hooks/projectListStore', () => ({
    subscribeProjectList: (_onChange: unknown, onEvent: (ev: unknown) => void) => {
        subscribers.add(onEvent);
        return () => {
            subscribers.delete(onEvent);
        };
    },
    getProjectList: () => rows,
}));

// jsdom has no Notification API; this fake records what the engine would post.
class FakeNotification {
    static permission: NotificationPermission = 'default';
    static instances: FakeNotification[] = [];
    title: string;
    options: NotificationOptions;
    closed = false;
    onclick: (() => void) | null = null;
    constructor(title: string, options?: NotificationOptions) {
        this.title = title;
        this.options = options ?? {};
        FakeNotification.instances.push(this);
    }
    close() {
        this.closed = true;
    }
}
vi.stubGlobal('Notification', FakeNotification);

const navigate = vi.fn();
const visible = (v: 'visible' | 'hidden') => Object.defineProperty(document, 'visibilityState', { value: v, configurable: true });
// jsdom has no window.focus; keep its "not implemented" noise out.
window.focus = () => undefined;

import {
    FINISHED_COOLDOWN_MS,
    classifyTransition,
    parseDecision,
    setNotificationUi,
    setNotificationsEnabled,
} from './engine';

const project = (id: string, main: string): ProjectSummary =>
    ({ id, type: 'project', name: `Proj ${id}`, main_thread_id: main }) as unknown as ProjectSummary;
const folder = (id: string, path: string): ProjectSummary =>
    ({ id, type: 'folder', name: id, path }) as unknown as ProjectSummary;
const thread = (over: Partial<Thread>): Thread =>
    ({ id: 't1', project_id: 'p1', status: 'working', title: 'A thread', archived_at: undefined, ...over }) as unknown as Thread;
const event = (over: Partial<ThreadEvent>): ThreadEvent =>
    ({ id: 'e1', kind: 'decision', content: '', payload: '', thread_id: 'm1', ...over }) as unknown as ThreadEvent;

const emit = (ev: unknown) => subscribers.forEach((s) => s(ev));

beforeEach(() => {
    FakeNotification.permission = 'granted';
    FakeNotification.instances.length = 0;
    rows = [project('p1', 'm1'), folder('f1', '/tmp/w')];
    setNotificationUi({
        t: (key) => key,
        navigate,
        paths: { project: (id) => `/p/${id}`, folder: (p) => `/f?path=${p}` },
    });
    visible('visible');
    setNotificationsEnabled(true);
});

afterEach(() => {
    setNotificationsEnabled(false);
    navigate.mockClear();
});

describe('classifyTransition', () => {
    it('asks on arrival at waiting or failed, and on a turn that ends', () => {
        expect(classifyTransition('working', 'waiting')).toBe('waiting');
        expect(classifyTransition('failed', 'waiting')).toBe('waiting');
        expect(classifyTransition('working', 'failed')).toBe('failed');
        expect(classifyTransition('working', 'ready')).toBe('finished');
        expect(classifyTransition('waiting', 'idle')).toBe('finished');
    });

    it('stays quiet within a state, for leaves, and without a baseline', () => {
        expect(classifyTransition('waiting', 'waiting')).toBeNull();
        expect(classifyTransition('failed', 'failed')).toBeNull();
        expect(classifyTransition('ready', 'idle')).toBeNull();
        expect(classifyTransition('working', 'working')).toBeNull();
        expect(classifyTransition(undefined, 'waiting')).toBeNull();
    });
});

describe('parseDecision', () => {
    it('keeps a card until it is answered and survives a broken payload', () => {
        expect(parseDecision(event({ payload: '{"question":"Which DB?"}' }))).toEqual({ question: 'Which DB?' });
        expect(parseDecision(event({ payload: '{"question":"Which DB?","chosen":1}' }))).toBeNull();
        expect(parseDecision(event({ payload: 'not json' }))).toBeNull();
        expect(parseDecision(event({ payload: undefined }))).toBeNull();
    });
});

describe('thread notifications', () => {
    it('baselines the first sight of a thread without notifying', () => {
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'which port?' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });

    it('notifies waiting with the reason when the tab is hidden, and routes the click', () => {
        emit({ type: 'thread', project_id: 'p1', data: thread({}) });
        visible('hidden');
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'Approve: rm -rf' }) });
        expect(FakeNotification.instances).toHaveLength(1);
        const n = FakeNotification.instances[0];
        expect(n.title).toBe('A thread');
        expect(n.options.body).toBe('Approve: rm -rf');
        expect(n.options.tag).toBe('tb-thread-t1');
        n.onclick?.();
        expect(navigate).toHaveBeenCalledWith('/p/p1?thread=t1');
    });

    it('a folder thread routes to the folder page with the session open', () => {
        emit({ type: 'thread', project_id: 'f1', data: thread({ id: 's1', project_id: 'f1' }) });
        visible('hidden');
        emit({ type: 'thread', project_id: 'f1', data: thread({ id: 's1', project_id: 'f1', status: 'waiting' }) });
        FakeNotification.instances[0].onclick?.();
        expect(navigate).toHaveBeenCalledWith('/f?path=/tmp/w&session=s1');
    });

    it('the main thread routes to the project page itself', () => {
        emit({ type: 'thread', project_id: 'p1', data: thread({ id: 'm1' }) });
        visible('hidden');
        emit({ type: 'thread', project_id: 'p1', data: thread({ id: 'm1', status: 'waiting', wait_reason: 'why' }) });
        FakeNotification.instances[0].onclick?.();
        expect(navigate).toHaveBeenCalledWith('/p/p1');
    });

    it('a visible tab stays quiet but keeps baselining', () => {
        emit({ type: 'thread', project_id: 'p1', data: thread({}) });
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'seen on screen' }) });
        expect(FakeNotification.instances).toHaveLength(0);
        // The same wait again changes nothing, so still quiet.
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'seen on screen' }) });
        expect(FakeNotification.instances).toHaveLength(0);
        visible('hidden');
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'failed', wait_reason: 'Failed: no' }) });
        expect(FakeNotification.instances).toHaveLength(1);
        expect(FakeNotification.instances[0].options.body).toBe('Failed: no');
    });

    it('finished reports the turn, at most once per cooldown', () => {
        vi.useFakeTimers({ now: 1_700_000_000_000 });
        try {
            emit({ type: 'thread', project_id: 'p1', data: thread({}) });
            visible('hidden');
            emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'ready', last_report: '## Done\nall green' }) });
            expect(FakeNotification.instances).toHaveLength(1);
            expect(FakeNotification.instances[0].options.body).toBe('## Done');
            vi.advanceTimersByTime(FINISHED_COOLDOWN_MS - 1);
            emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'working' }) });
            emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'ready', last_report: 'again' }) });
            expect(FakeNotification.instances).toHaveLength(1);
            vi.advanceTimersByTime(FINISHED_COOLDOWN_MS);
            emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'working' }) });
            emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'ready', last_report: 'again' }) });
            expect(FakeNotification.instances).toHaveLength(2);
        } finally {
            vi.useRealTimers();
        }
    });

    it('archived threads never notify', () => {
        emit({ type: 'thread', project_id: 'p1', data: thread({}) });
        visible('hidden');
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'failed', archived_at: '2026-01-01T00:00:00Z' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });
});

describe('decision notifications', () => {
    it('names the project, asks the question, and closes when answered', () => {
        visible('hidden');
        emit({ type: 'thread_event', project_id: 'p1', data: event({ id: 'd1', payload: '{"question":"Which DB?"}' }) });
        expect(FakeNotification.instances).toHaveLength(1);
        const n = FakeNotification.instances[0];
        expect(n.title).toBe('Proj p1');
        expect(n.options.body).toBe('Which DB?');
        expect(n.options.tag).toBe('tb-decision-d1');
        emit({ type: 'thread_event', project_id: 'p1', data: event({ id: 'd1', payload: '{"question":"Which DB?","chosen":0}' }) });
        expect(n.closed).toBe(true);
    });

    it('an already-answered card never notifies', () => {
        visible('hidden');
        emit({ type: 'thread_event', project_id: 'p1', data: event({ payload: '{"question":"Which DB?","chosen":2}' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });

    it('ignores every other transcript entry', () => {
        visible('hidden');
        emit({ type: 'thread_event', project_id: 'p1', data: event({ kind: 'approval', payload: '{"state":"pending"}' }) });
        emit({ type: 'thread_event', project_id: 'p1', data: event({ kind: 'tool_result' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });
});

describe('the switch', () => {
    it('off, nothing notifies even hidden', () => {
        visible('hidden');
        setNotificationsEnabled(false);
        FakeNotification.permission = 'granted';
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'no' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });

    it('revoked permission turns it off silently', () => {
        FakeNotification.permission = 'denied';
        setNotificationsEnabled(true);
        visible('hidden');
        emit({ type: 'thread', project_id: 'p1', data: thread({ status: 'waiting', wait_reason: 'no' }) });
        expect(FakeNotification.instances).toHaveLength(0);
    });
});
