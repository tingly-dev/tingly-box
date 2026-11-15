import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { coalescedFetch } from './coalescedFetch';
import { readCached } from './readCache';

describe('readCached', () => {
    beforeEach(() => vi.useFakeTimers());
    afterEach(() => {
        vi.useRealTimers();
        vi.unstubAllGlobals();
    });

    it('serves a repeat read from the stored copy and hands out independent copies', async () => {
        const load = vi.fn(async () => ({ success: true, data: [3, 1, 2] }));
        const a = await readCached('k-copy', load);
        a.data.sort();
        const b = await readCached('k-copy', load);

        expect(load).toHaveBeenCalledTimes(1);
        expect(b.data).toEqual([3, 1, 2]);
    });

    it('reads again once the copy is old', async () => {
        const load = vi.fn(async () => ({ success: true }));
        await readCached('k-age', load);
        vi.advanceTimersByTime(6_000);
        await readCached('k-age', load);
        expect(load).toHaveBeenCalledTimes(2);
    });

    it('reads again when asked for a fresh copy', async () => {
        const load = vi.fn(async () => ({ success: true }));
        await readCached('k-fresh', load);
        await readCached('k-fresh', load, { fresh: true });
        expect(load).toHaveBeenCalledTimes(2);
    });

    it('does not serve a copy taken before a write', async () => {
        vi.stubGlobal('fetch', vi.fn(async () => Response.json({})));
        const load = vi.fn(async () => ({ success: true }));
        await readCached('k-write', load);
        await coalescedFetch('https://x.test/api/v1/thing', { method: 'PUT', body: '{}' });
        await readCached('k-write', load);
        expect(load).toHaveBeenCalledTimes(2);
    });

    it('does not keep a failed read', async () => {
        const load = vi.fn(async () => ({ success: false, error: 'down' }));
        await readCached('k-fail', load);
        await readCached('k-fail', load);
        expect(load).toHaveBeenCalledTimes(2);
    });
});
