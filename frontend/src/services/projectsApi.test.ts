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

const respond = (projects: unknown, workdirs: unknown) =>
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
        const path = new URL((input as Request).url).pathname;
        const body = path.endsWith('/workdirs') ? { workdirs } : { projects };
        return new Response(JSON.stringify(body), { status: 200 });
    });

describe('anythingWaiting', () => {
    afterEach(() => vi.restoreAllMocks());

    it('is true for a waiting project thread', async () => {
        respond([{ threads: { waiting: 1 } }], [{ waiting: 0 }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is true for a waiting folder session alone', async () => {
        respond([{ threads: { waiting: 0 } }], [{ waiting: 2 }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is false when nothing waits, or a list is unreachable', async () => {
        respond([], []);
        expect(await anythingWaiting()).toBe(false);
        vi.restoreAllMocks();
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 503 }));
        expect(await anythingWaiting()).toBe(false);
    });
});
