import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// getApiBaseUrl() is read per request; a test can point it away from the page
// origin to take projectsFetch's desktop (cross-origin) branch.
const site = vi.hoisted(() => ({ base: 'http://localhost:3000' }));

vi.mock('./openapi', () => ({ getControlApiHeaders: async () => ({ Authorization: 'Bearer test' }) }));
vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: async () => site.base }));

import { anythingWaiting, projectsFetch } from './projectsApi';

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

beforeEach(() => {
    // The page origin of this jsdom window; the browser-tab (same-origin) case.
    site.base = window.location.origin;
});

afterEach(() => vi.restoreAllMocks());

describe('anythingWaiting', () => {
    it('is true for a waiting project thread', async () => {
        respond([{ type: 'project', threads: { waiting: 1 } }, { type: 'folder', threads: { waiting: 0 } }]);
        expect(await anythingWaiting()).toBe(true);
    });

    it('is true for a waiting folder thread alone', async () => {
        respond([{ type: 'project', threads: { waiting: 0 } }, { type: 'folder', threads: { waiting: 2 } }]);
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

describe('projectsFetch cross-origin body', () => {
    const sentCall = (fetchSpy: { mock: { calls: unknown[][] } }) => {
        const [url, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
        return { url, headers: new Headers(init.headers), body: init.body, method: init.method };
    };

    it('replays a FormData upload as-is instead of buffering it', async () => {
        site.base = 'http://127.0.0.1:12580';
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }));
        const arrayBuffer = vi.spyOn(NodeRequest.prototype, 'arrayBuffer');

        const form = new FormData();
        form.append('files', new File([new Uint8Array(1 << 20)], 'big.bin', { type: 'application/octet-stream' }), 'big.bin');
        await projectsFetch('/api/v1/projects/p1/library?x=1', { method: 'POST', body: form });

        const sent = sentCall(fetchSpy);
        expect(sent.url).toBe('http://127.0.0.1:12580/api/v1/projects/p1/library?x=1');
        expect(sent.method).toBe('POST');
        // The same body object: the File is streamed by fetch, never read into
        // the heap, so a large upload costs one copy instead of two.
        expect(sent.body).toBe(form);
        expect(arrayBuffer).not.toHaveBeenCalled();
        // The multipart boundary is regenerated for the new serialization.
        expect(sent.headers.has('content-type')).toBe(false);
        expect(sent.headers.get('authorization')).toBe('Bearer test');
    });

    it('keeps a caller-set Content-Type on a FormData body', async () => {
        site.base = 'http://127.0.0.1:12580';
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }));
        const form = new FormData();
        form.append('files', new Blob(['x']), 'x.txt');
        await projectsFetch('/api/v1/projects/p1/attachments', {
            method: 'POST',
            headers: { 'Content-Type': 'multipart/form-data' },
            body: form,
        });
        expect(sentCall(fetchSpy).headers.get('content-type')).toBe('multipart/form-data');
    });

    it('passes a small JSON string body through, headers unchanged', async () => {
        site.base = 'http://127.0.0.1:12580';
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }));
        const payload = JSON.stringify({ path: '/tmp/x' });
        await projectsFetch('/api/v1/projects/directories', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: payload,
        });
        const sent = sentCall(fetchSpy);
        expect(sent.body).toBe(payload);
        expect(sent.headers.get('content-type')).toBe('application/json');
    });

    it('buffers a Request input (the typed client), preserving method, headers and body', async () => {
        site.base = 'http://127.0.0.1:12580';
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}', { status: 200 }));
        // openapi-fetch hands projectsFetch a Request, not (url, init).
        const req = new Request('http://localhost:3000/api/v1/projects', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: 't' }),
        });
        await projectsFetch(req);

        const sent = sentCall(fetchSpy);
        expect(sent.method).toBe('POST');
        expect(sent.headers.get('content-type')).toBe('application/json');
        expect(sent.body).toBeInstanceOf(ArrayBuffer);
        expect(new TextDecoder().decode(sent.body as ArrayBuffer)).toBe('{"name":"t"}');
    });

    it('sends no body for GET', async () => {
        site.base = 'http://127.0.0.1:12580';
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('[]', { status: 200 }));
        await projectsFetch('/api/v1/projects');
        expect(sentCall(fetchSpy).body).toBeUndefined();
    });
});
