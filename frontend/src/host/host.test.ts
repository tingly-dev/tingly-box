import { afterEach, describe, expect, it, vi } from 'vitest';
import { host } from './index';

// jsdom serves the page from http://localhost, so ./index picks the browser
// bridge — exactly what a real tab does. These pin the browser behaviour the
// rest of the app relies on: same-origin API, no shell token, external links
// in a new tab. The desktop bridge is covered by desktop.test.ts.
describe('browser host bridge', () => {
    afterEach(() => vi.restoreAllMocks());

    it('uses the page origin as the gateway', async () => {
        expect(host.kind).toBe('browser');
        expect(await host.gatewayPort()).toBeNull();
        expect(await host.shellAuthToken()).toBeNull();
    });

    it('opens external links in a new tab without an opener', () => {
        const open = vi.spyOn(window, 'open').mockReturnValue(null);
        host.openExternal('https://github.com/tingly-dev/tingly-box');
        expect(open).toHaveBeenCalledWith('https://github.com/tingly-dev/tingly-box', '_blank', 'noopener,noreferrer');
    });
});
