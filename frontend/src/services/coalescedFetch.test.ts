import { afterEach, describe, expect, it, vi } from 'vitest';
import { coalescedFetch } from './coalescedFetch';

const deferred = () => {
    let resolve!: (r: Response) => void;
    const promise = new Promise<Response>((res) => { resolve = res; });
    return { promise, resolve };
};

describe('coalescedFetch', () => {
    afterEach(() => vi.unstubAllGlobals());

    it('shares one request between concurrent identical GETs, each with its own body', async () => {
        const d = deferred();
        const fetchMock = vi.fn(() => d.promise);
        vi.stubGlobal('fetch', fetchMock);

        const a = coalescedFetch('https://x.test/api/v1/config', { headers: { Authorization: 'Bearer t' } });
        const b = coalescedFetch('https://x.test/api/v1/config', { headers: { Authorization: 'Bearer t' } });
        d.resolve(Response.json({ n: 1 }));

        const [ra, rb] = await Promise.all([a, b]);
        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(await ra.json()).toEqual({ n: 1 });
        expect(await rb.json()).toEqual({ n: 1 });
    });

    it('does not share across different URLs or credentials', async () => {
        const fetchMock = vi.fn(async () => Response.json({}));
        vi.stubGlobal('fetch', fetchMock);

        await Promise.all([
            coalescedFetch('https://x.test/a', { headers: { Authorization: 'Bearer 1' } }),
            coalescedFetch('https://x.test/a', { headers: { Authorization: 'Bearer 2' } }),
            coalescedFetch('https://x.test/b', { headers: { Authorization: 'Bearer 1' } }),
        ]);
        expect(fetchMock).toHaveBeenCalledTimes(3);
    });

    it('is not a cache: a GET after the first settled goes to the network again', async () => {
        const fetchMock = vi.fn(async () => Response.json({}));
        vi.stubGlobal('fetch', fetchMock);

        await coalescedFetch('https://x.test/a');
        await coalescedFetch('https://x.test/a');
        expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    it('a mutation in between keeps later reads from joining an older in-flight read', async () => {
        const stale = deferred();
        const fetchMock = vi.fn()
            .mockImplementationOnce(() => stale.promise)
            .mockImplementation(async () => Response.json({ fresh: true }));
        vi.stubGlobal('fetch', fetchMock);

        const before = coalescedFetch('https://x.test/a');
        await coalescedFetch('https://x.test/a', { method: 'POST', body: '{}' });
        const after = coalescedFetch('https://x.test/a');
        stale.resolve(Response.json({ fresh: false }));

        expect(await (await before).json()).toEqual({ fresh: false });
        expect(await (await after).json()).toEqual({ fresh: true });
    });

    it("one caller's abort rejects only that caller", async () => {
        const d = deferred();
        vi.stubGlobal('fetch', vi.fn(() => d.promise));

        const ctrl = new AbortController();
        const aborted = coalescedFetch('https://x.test/a', { signal: ctrl.signal });
        const other = coalescedFetch('https://x.test/a');
        ctrl.abort();
        d.resolve(Response.json({ ok: true }));

        await expect(aborted).rejects.toBeDefined();
        expect(await (await other).json()).toEqual({ ok: true });
    });
});
