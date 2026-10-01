import { describe, expect, it, vi } from 'vitest';
import { BOUND_METHODS, createDesktopHost, OPEN_URL_ROUTE, SAVE_FILE_ROUTE } from './desktop';

// A stand-in for @wailsio/runtime: just the two namespaces the bridge uses.
function fakeRuntime() {
    const runtime = {
        Call: { ByName: vi.fn(async (name: string): Promise<unknown> => (name === BOUND_METHODS.getPort ? 12580 : 'tb-token')) },
        Browser: { OpenURL: vi.fn(async () => {}) },
    };
    // The bridge only touches these members; the cast stands in for the
    // full runtime module type.
    const load = () => Promise.resolve(runtime as unknown as typeof import('@wailsio/runtime'));
    return { runtime, load };
}

describe('desktop host bridge', () => {
    it('loads the runtime at creation, not lazily on first call', () => {
        const load = vi.fn(fakeRuntime().load);
        createDesktopHost(load);
        expect(load).toHaveBeenCalledTimes(1);
    });

    it('reaches the Go service by fully-qualified method name', async () => {
        const { runtime, load } = fakeRuntime();
        const host = createDesktopHost(load);
        expect(host.kind).toBe('desktop');
        expect(await host.gatewayPort()).toBe(12580);
        expect(await host.shellAuthToken()).toBe('tb-token');
        await host.openMainWindow('/dashboard');
        expect(runtime.Call.ByName).toHaveBeenCalledWith(BOUND_METHODS.openMainWindow, '/dashboard');
    });

    it('treats an empty shell token as none', async () => {
        const { runtime, load } = fakeRuntime();
        runtime.Call.ByName.mockResolvedValue('');
        expect(await createDesktopHost(load).shellAuthToken()).toBeNull();
    });

    it('opens external links through the shell route', async () => {
        const { runtime, load } = fakeRuntime();
        const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{"success":true}'));
        createDesktopHost(load).openExternal('https://github.com/tingly-dev/tingly-box');
        await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
        const [url, init] = fetchMock.mock.calls[0];
        expect(url).toBe(`http://localhost:12580${OPEN_URL_ROUTE}`);
        expect(init).toMatchObject({
            method: 'POST',
            headers: { Authorization: 'Bearer tb-token', 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: 'https://github.com/tingly-dev/tingly-box' }),
        });
        expect(runtime.Browser.OpenURL).not.toHaveBeenCalled();
        fetchMock.mockRestore();
    });

    it('falls back to the runtime when the shell route fails', async () => {
        const { runtime, load } = fakeRuntime();
        const fetchMock = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new TypeError('network'));
        vi.spyOn(console, 'error').mockImplementation(() => {});
        createDesktopHost(load).openExternal('https://example.com');
        await vi.waitFor(() => expect(runtime.Browser.OpenURL).toHaveBeenCalledWith('https://example.com'));
        fetchMock.mockRestore();
        vi.restoreAllMocks();
    });

    it('saves through the shell route with the shell token', async () => {
        const { load } = fakeRuntime();
        const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{"saved":true}'));
        const anchor = vi.spyOn(document, 'createElement');
        const blob = new Blob(['x']);
        createDesktopHost(load).saveFile(blob, 'slices.zip');
        await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
        const [url, init] = fetchMock.mock.calls[0];
        expect(url).toBe(`http://localhost:12580${SAVE_FILE_ROUTE}?name=slices.zip`);
        expect(init).toMatchObject({ method: 'POST', headers: { Authorization: 'Bearer tb-token' }, body: blob });
        expect(anchor).not.toHaveBeenCalledWith('a');
        fetchMock.mockRestore();
        anchor.mockRestore();
    });

    it('falls back to a download link only when the request fails', async () => {
        const { load } = fakeRuntime();
        const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('', { status: 500 }));
        vi.spyOn(console, 'error').mockImplementation(() => {});
        const createObjectURL = vi.fn(() => 'blob:x');
        Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
        createDesktopHost(load).saveFile(new Blob(['x']), 'a.txt');
        await vi.waitFor(() => expect(createObjectURL).toHaveBeenCalled());
        fetchMock.mockRestore();
        vi.restoreAllMocks();
    });
});
