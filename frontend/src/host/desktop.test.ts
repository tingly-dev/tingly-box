import { describe, expect, it, vi } from 'vitest';
import { BOUND_METHODS, createDesktopHost } from './desktop';

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

    it('opens external links in the OS browser', async () => {
        const { runtime, load } = fakeRuntime();
        createDesktopHost(load).openExternal('https://github.com/tingly-dev/tingly-box');
        await Promise.resolve();
        await Promise.resolve();
        expect(runtime.Browser.OpenURL).toHaveBeenCalledWith('https://github.com/tingly-dev/tingly-box');
    });
});
