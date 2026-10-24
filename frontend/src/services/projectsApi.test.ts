import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('./openapi', () => ({ getControlApiHeaders: async () => ({}) }));
vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: async () => window.location.origin }));

import { anythingWaiting } from './projectsApi';

// Node's Request does not resolve a relative URL against the page as a
// browser does.
const NodeRequest = globalThis.Request;
vi.stubGlobal('Request', class extends NodeRequest {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
        super(typeof input === 'string' ? new URL(input, window.location.origin) : input, init);
    }
});

const respond = (projects: unknown) =>
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify({ projects }), { status: 200 }));

describe('anythingWaiting', () => {
    afterEach(() => vi.restoreAllMocks());

    it('is true for a waiting project thread', async () => {
        respond([{ type: 'project', threads: { waiting: 1 } }, { type: 'folder', threads: { waiting: 0 } }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is true for a waiting folder thread alone', async () => {
        respond([{ type: 'project', threads: { waiting: 0 } }, { type: 'folder', threads: { waiting: 2 } }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is true for a failed thread even when none are waiting', async () => {
        respond([{ type: 'project', threads: { waiting: 0, failed: 1 } }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is false when nothing waits, or the list is unreachable', async () => {
        respond([]);
        expect(await anythingWaiting()).toBe(false);
        vi.restoreAllMocks();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 503 }));
        expect(await anythingWaiting()).toBe(false);
    });
});
