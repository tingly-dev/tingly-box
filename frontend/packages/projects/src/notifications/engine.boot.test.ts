import { beforeEach, describe, expect, it, vi } from 'vitest';

// The engine arms itself when its module is first imported, so each test
// loads a fresh copy: the page after a reload, with the permission already
// granted and the user's choice stored.
const subscribers = new Set<(ev: unknown) => void>();
vi.mock('../hooks/projectListStore', () => ({
    subscribeProjectList: (_onChange: unknown, onEvent: (ev: unknown) => void) => {
        subscribers.add(onEvent);
        return () => {
            subscribers.delete(onEvent);
        };
    },
    getProjectList: () => [],
}));

class FakeNotification {
    static permission: NotificationPermission = 'granted';
    static instances: FakeNotification[] = [];
    onclick: (() => void) | null = null;
    constructor(public title: string, public options?: NotificationOptions) {
        FakeNotification.instances.push(this);
    }
    close() {}
}

beforeEach(() => {
    vi.resetModules();
    subscribers.clear();
    FakeNotification.permission = 'granted';
    FakeNotification.instances.length = 0;
    vi.stubGlobal('Notification', FakeNotification);
    window.localStorage.removeItem('tingly-project.notifications');
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true });
    document.hasFocus = () => false;
});

describe('the engine at page load', () => {
    it('is already watching when the permission is granted and the user has not turned it off', async () => {
        await import('./engine');
        expect(subscribers.size).toBe(1);
        const emit = (ev: unknown) => subscribers.forEach((s) => s(ev));
        emit({ type: 'thread', project_id: 'p1', data: { id: 't1', project_id: 'p1', status: 'working', title: 'T' } });
        emit({ type: 'thread', project_id: 'p1', data: { id: 't1', project_id: 'p1', status: 'waiting', title: 'T', wait_reason: 'Approve?' } });
        expect(FakeNotification.instances).toHaveLength(1);
    });

    it('stays off when the user turned it off', async () => {
        window.localStorage.setItem('tingly-project.notifications', '0');
        await import('./engine');
        expect(subscribers.size).toBe(0);
    });

    it('stays off until the browser grants', async () => {
        FakeNotification.permission = 'default';
        await import('./engine');
        expect(subscribers.size).toBe(0);
    });
});
