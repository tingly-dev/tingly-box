import { beforeEach, describe, expect, it, vi } from 'vitest';

const calls: Array<{ method: string; body?: unknown }> = [];
let remotePrefs: Record<string, unknown> = {};

vi.mock('./openapi', () => ({
    controlApi: async (request: (client: unknown, headers: Record<string, string>) => Promise<any>) => {
        const client = {
            GET: async () => {
                calls.push({ method: 'GET' });
                return { data: { success: true, prefs: remotePrefs } };
            },
            PATCH: async (_path: string, opts: { body: unknown }) => {
                calls.push({ method: 'PATCH', body: opts.body });
                return { data: { success: true } };
            },
        };
        const res = await request(client, {});
        return res.data;
    },
}));

const { isSyncedKey, setSyncedItem, removeSyncedItem, syncUiPrefs } = await import('./uiPrefs');

describe('ui prefs sync', () => {
    beforeEach(() => {
        localStorage.clear();
        calls.length = 0;
        remotePrefs = {};
    });

    it('mirrors only the listed keys', () => {
        expect(isSyncedKey('scenario.hiddenScenarios')).toBe(true);
        expect(isSyncedKey('scenario.hiddenDefaultsVersion')).toBe(true);
        expect(isSyncedKey('setup-card-step2-done-codex')).toBe(false);
        expect(isSyncedKey('user_auth_token')).toBe(false);
    });

    it('prefers the server copy and notifies storage listeners', async () => {
        localStorage.setItem('scenario.hiddenScenarios', '["pi"]');
        remotePrefs = { 'scenario.hiddenScenarios': '["cursor"]' };
        const seen: Array<string | null> = [];
        const onStorage = (e: StorageEvent) => seen.push(e.key);
        window.addEventListener('storage', onStorage);

        await syncUiPrefs();
        window.removeEventListener('storage', onStorage);

        expect(localStorage.getItem('scenario.hiddenScenarios')).toBe('["cursor"]');
        expect(seen).toEqual(['scenario.hiddenScenarios']);
        expect(calls.filter(c => c.method === 'PATCH')).toEqual([]);
    });

    it('uploads values only this surface has, and nothing unrelated', async () => {
        localStorage.setItem('scenario.hiddenDefaultsVersion', '3');
        localStorage.setItem('user_auth_token', 'secret');

        await syncUiPrefs();

        expect(calls).toContainEqual({ method: 'PATCH', body: { prefs: { 'scenario.hiddenDefaultsVersion': '3' } } });
    });

    it('writes through on set and remove', () => {
        setSyncedItem('scenario.hiddenScenarios', '[]');
        removeSyncedItem('scenario.hiddenDefaultsVersion');
        setSyncedItem('not-synced', 'x');

        expect(localStorage.getItem('scenario.hiddenScenarios')).toBe('[]');
        expect(localStorage.getItem('not-synced')).toBe('x');
        const bodies = calls.filter(c => c.method === 'PATCH').map(c => c.body);
        expect(bodies).toEqual([
            { prefs: { 'scenario.hiddenScenarios': '[]' } },
            { prefs: { 'scenario.hiddenDefaultsVersion': null } },
        ]);
    });
});
